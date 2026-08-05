package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func runUninstall(t *testing.T, h *home, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Uninstall(args, h.env, &out, &errb)
	return out.String(), errb.String(), code
}

// TestUninstallRemovesOnlyTheKey is §9.3: "uninstall removes only the
// statusLine key and leaves every other byte, and every later user edit,
// intact."
//
// The "later user edit" half is what makes this more than a delete test. The
// sequence is: install, then the user edits something unrelated, then
// uninstall. A tool that restored its own backup at this point would silently
// undo that edit — which is exactly why §10.3 says it must not.
func TestUninstallRemovesOnlyTheKey(t *testing.T) {
	h := newHome(t)
	h.seed(t, "plain.json")

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("init: exit %d: %s", code, errOut)
	}

	// The later user edit: a hook they added after installing.
	installed := h.read(t)
	edited, err := sjson.SetBytes(installed, "hooks.PreToolUse", []any{map[string]any{"matcher": "Bash"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.settings, edited, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, errOut, code := runUninstall(t, h); code != 0 {
		t.Fatalf("uninstall: exit %d: %s", code, errOut)
	}

	final := h.read(t)
	if gjson.GetBytes(final, "statusLine").Exists() {
		t.Errorf("statusLine survived:\n%s", final)
	}
	if !gjson.GetBytes(final, "hooks.PreToolUse").Exists() {
		t.Errorf("the edit made after installing was reverted:\n%s", final)
	}
	if got := gjson.GetBytes(final, "theme").String(); got != "dark" {
		t.Errorf("theme = %q; unrelated keys must survive", got)
	}
	if got := gjson.GetBytes(final, "permissions.allow.0").String(); got != "Bash(git status)" {
		t.Errorf("permissions were lost: %q", got)
	}
}

// TestUninstallRoundTripsToTheOriginal is the strongest form of "leaves every
// other byte": install then uninstall must give back exactly the bytes we
// started with, trailing newline included.
func TestUninstallRoundTripsToTheOriginal(t *testing.T) {
	for _, corpus := range []string{"plain.json", "big-int.json", "unsorted-keys.json"} {
		t.Run(corpus, func(t *testing.T) {
			h := newHome(t)
			h.seed(t, corpus)
			before := h.read(t)

			if _, errOut, code := runInit(t, h); code != 0 {
				t.Fatalf("init: exit %d: %s", code, errOut)
			}
			if _, errOut, code := runUninstall(t, h); code != 0 {
				t.Fatalf("uninstall: exit %d: %s", code, errOut)
			}
			if after := h.read(t); !bytes.Equal(before, after) {
				t.Errorf("round trip changed the file\nbefore: %q\n after: %q", before, after)
			}
		})
	}
}

func TestUninstallBacksUpBeforeRemoving(t *testing.T) {
	h := newHome(t)
	h.seed(t, "plain.json")
	if _, _, code := runInit(t, h); code != 0 {
		t.Fatal("init failed")
	}
	installed := h.read(t)

	if _, errOut, code := runUninstall(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	backups := h.backups(t)
	if len(backups) == 0 {
		t.Fatal("no backup was made before removing the key")
	}
	// The newest backup is the state just before the removal, not the state
	// before the original install.
	newest, err := os.ReadFile(backups[len(backups)-1])
	if err != nil {
		t.Fatal(err)
	}
	if !gjson.GetBytes(newest, "statusLine").Exists() {
		t.Errorf("the backup does not contain what was removed:\n%s", newest)
	}
	_ = installed
}

func TestUninstallWithNothingInstalled(t *testing.T) {
	h := newHome(t)
	h.seed(t, "plain.json")
	before := h.read(t)

	out, errOut, code := runUninstall(t, h)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !bytes.Equal(before, h.read(t)) {
		t.Error("a no-op uninstall modified the file")
	}
	if len(h.backups(t)) != 0 {
		t.Error("a no-op uninstall made a backup")
	}
	if !strings.Contains(out, "nothing to do") {
		t.Errorf("expected a clear no-op message:\n%s", out)
	}
}

// TestUninstallLeavesSomebodyElsesStatusLine is not in §10.3. It is the
// difference between "uninstall me" and "uninstall whatever is there": a user
// who went back to another status line and then tidied up should not lose it.
func TestUninstallLeavesSomebodyElsesStatusLine(t *testing.T) {
	h := newHome(t)
	h.seed(t, "existing-statusline.json")
	before := h.read(t)

	out, errOut, code := runUninstall(t, h)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !bytes.Equal(before, h.read(t)) {
		t.Errorf("removed a status line belonging to another program:\n%s", h.read(t))
	}
	if !strings.Contains(out, "left alone") {
		t.Errorf("expected an explanation:\n%s", out)
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("the escape hatch must be named:\n%s", out)
	}

	// --force is that escape hatch.
	if _, errOut, code := runUninstall(t, h, "--force"); code != 0 {
		t.Fatalf("--force: exit %d: %s", code, errOut)
	}
	if gjson.GetBytes(h.read(t), "statusLine").Exists() {
		t.Error("--force did not remove it")
	}
}

func TestUninstallDeclinesOnComments(t *testing.T) {
	h := newHome(t)
	h.seed(t, "line-comments.json")
	before := h.read(t)

	out, _, code := runUninstall(t, h)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !bytes.Equal(before, h.read(t)) {
		t.Error("edited a file with comments in it")
	}
	if !strings.Contains(out, "not plain JSON") {
		t.Errorf("expected a refusal:\n%s", out)
	}
}

func TestUninstallLeavesTheConfigFile(t *testing.T) {
	h := newHome(t)
	if _, _, code := runInit(t, h); code != 0 {
		t.Fatal("init failed")
	}
	if _, _, code := runUninstall(t, h); code != 0 {
		t.Fatal("uninstall failed")
	}
	if _, err := os.Stat(h.config); err != nil {
		t.Errorf("the config file was deleted: %v", err)
	}
}

func TestMentionsThisBinary(t *testing.T) {
	cases := map[string]bool{
		`{"type":"command","command":"/usr/local/bin/cc-statusline render"}`:            true,
		`{"type":"command","command":"'/opt/App Support/cc-statusline' render"}`:        true,
		`{"type":"command","command":"/home/u/.local/bin/cc-statusline spike capture"}`: true,
		`{"type":"command","command":"/usr/bin/starship prompt"}`:                       false,
		`{"type":"command","command":"npx ccstatusline"}`:                               false,
		`{"type":"command","command":""}`:                                               false,
		`"not an object"`:                                                               false,
	}
	for raw, want := range cases {
		if got := mentionsThisBinary(raw); got != want {
			t.Errorf("mentionsThisBinary(%s) = %v, want %v", raw, got, want)
		}
	}
}
