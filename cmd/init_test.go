package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

// PRD §9.3's install criteria, as integration tests against a temporary HOME.
//
// Every one of these runs the real Init against real files. The only thing
// stubbed is os.Executable, because a test asserting that the command contains
// the test binary's own name would be asserting on the toolchain rather than on
// this code.

const fakeExe = "/opt/cc-statusline/bin/cc-statusline"

// home builds an isolated environment: a HOME with a .claude directory, an
// XDG_CONFIG_HOME, and nothing else. `env -i` conditions plus the two variables
// under test, so nothing on the developer's machine can leak into a result.
type home struct {
	dir      string
	env      map[string]string
	settings string
	config   string
}

func newHome(t *testing.T) *home {
	t.Helper()
	dir := t.TempDir()
	h := &home{
		dir:      dir,
		settings: filepath.Join(dir, ".claude", "settings.json"),
		config:   filepath.Join(dir, ".config", "cc-statusline", "config.toml"),
		env: map[string]string{
			"HOME":            dir,
			"XDG_CONFIG_HOME": filepath.Join(dir, ".config"),
		},
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}

	old := executable
	executable = func() (string, error) { return fakeExe, nil }
	t.Cleanup(func() { executable = old })
	return h
}

// seed installs a corpus file as this home's settings.json.
func (h *home) seed(t *testing.T, corpus string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "settings", "testdata", corpus))
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	if err := os.WriteFile(h.settings, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (h *home) read(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(h.settings)
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	return raw
}

func (h *home) backups(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(h.settings + ".bak-*")
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func runInit(t *testing.T, h *home, args ...string) (string, string, int) {
	t.Helper()
	var out, errb bytes.Buffer
	code := Init(args, h.env, &out, &errb)
	return out.String(), errb.String(), code
}

func TestInitOnAFreshMachine(t *testing.T) {
	h := newHome(t)

	out, errOut, code := runInit(t, h)
	if code != 0 {
		t.Fatalf("exit %d\nstderr: %s", code, errOut)
	}
	if errOut != "" {
		t.Errorf("stderr: %s", errOut)
	}

	raw := h.read(t)
	sl := gjson.GetBytes(raw, "statusLine")
	if !sl.Exists() {
		t.Fatalf("no statusLine written:\n%s", raw)
	}
	if got := sl.Get("command").String(); got != fakeExe+" render" {
		t.Errorf("command = %q, want %q", got, fakeExe+" render")
	}
	if got := sl.Get("type").String(); got != "command" {
		t.Errorf("type = %q", got)
	}
	// §10.2's rationale: 60 matches the duration segment's own granularity, so
	// every tick either changes the display or does nothing observable.
	if got := sl.Get("refreshInterval").Int(); got != 60 {
		t.Errorf("refreshInterval = %d, want 60", got)
	}
	if sl.Get("padding").Exists() {
		t.Error("padding was invented where the user set none")
	}

	if _, err := os.Stat(h.config); err != nil {
		t.Errorf("config.toml was not written: %v", err)
	}
	if !strings.Contains(out, "uninstall") {
		t.Error("step 7 must print how to undo this")
	}
	// No file existed, so there was nothing to back up.
	if got := h.backups(t); len(got) != 0 {
		t.Errorf("backed up a file that did not exist: %v", got)
	}
}

// TestInitIsIdempotent is §9.3: "init run twice produces no second backup and
// no file modification."
func TestInitIsIdempotent(t *testing.T) {
	h := newHome(t)
	h.seed(t, "plain.json")

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("first init: exit %d, %s", code, errOut)
	}
	first := h.read(t)
	backupsAfterFirst := len(h.backups(t))

	out, errOut, code := runInit(t, h)
	if code != 0 {
		t.Fatalf("second init: exit %d, %s", code, errOut)
	}
	if !bytes.Equal(first, h.read(t)) {
		t.Errorf("the second run modified the file\nbefore: %s\n after: %s", first, h.read(t))
	}
	if got := len(h.backups(t)); got != backupsAfterFirst {
		t.Errorf("second run made a backup: %d then %d", backupsAfterFirst, got)
	}
	if !strings.Contains(out, "already installed") {
		t.Errorf("expected the run to say it was a no-op:\n%s", out)
	}
}

// TestInitDeclinesOnComments is §9.3: "a file with comments causes init to
// decline and print manual instructions."
func TestInitDeclinesOnComments(t *testing.T) {
	for _, corpus := range []string{"line-comments.json", "block-comments.json", "trailing-comma.json"} {
		t.Run(corpus, func(t *testing.T) {
			h := newHome(t)
			h.seed(t, corpus)
			before := h.read(t)

			out, errOut, code := runInit(t, h)
			if code != 0 {
				t.Fatalf("declining is not a failure; got exit %d: %s", code, errOut)
			}
			if !bytes.Equal(before, h.read(t)) {
				t.Error("the file was modified despite the refusal")
			}
			if got := h.backups(t); len(got) != 0 {
				t.Errorf("backed up a file it then refused to edit: %v", got)
			}

			// The remedy has to be usable: a block the user can paste.
			if !strings.Contains(out, `"statusLine"`) {
				t.Errorf("no manual block printed:\n%s", out)
			}
			block := out[strings.Index(out, `  "statusLine"`):]
			block = block[:strings.LastIndex(block, "}")+1]
			if !gjson.Valid("{\n" + block + "\n}") {
				t.Errorf("the printed block does not paste into an object:\n%s", block)
			}
		})
	}
}

// TestInitDeclinesRatherThanWriteIntoAComment is C-1's finding as an install
// test. This is the case §10.2's own step order would have got wrong: the
// commented-out statusLine is found by gjson, so an equality check performed
// before the plain-JSON check reports "already installed" and exits happy.
func TestInitDeclinesRatherThanWriteIntoAComment(t *testing.T) {
	h := newHome(t)
	h.seed(t, "commented-out-statusline.json")
	before := h.read(t)

	out, _, code := runInit(t, h)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !bytes.Equal(before, h.read(t)) {
		t.Errorf("the comment was edited:\n%s", h.read(t))
	}
	if strings.Contains(out, "already installed") {
		t.Errorf("reported success against a value read out of a comment:\n%s", out)
	}
	if !strings.Contains(out, "not plain JSON") {
		t.Errorf("expected a refusal:\n%s", out)
	}
}

// TestInitLeavesSlashesInStringsAlone is §9.3: "`//` inside a string literal is
// not treated as a comment."
func TestInitLeavesSlashesInStringsAlone(t *testing.T) {
	h := newHome(t)
	h.seed(t, "slashes-in-string.json")

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	raw := h.read(t)
	if !gjson.GetBytes(raw, "statusLine").Exists() {
		t.Fatalf("a URL containing // blocked the install:\n%s", raw)
	}
	if got := gjson.GetBytes(raw, "docs").String(); got != "see https://example.com/// for details" {
		t.Errorf("the URL was mangled: %q", got)
	}
}

// TestInitPreservesEveryOtherByte is §9.3: "non-alphabetical key order and a
// 17-digit integer survive byte-identical."
func TestInitPreservesEveryOtherByte(t *testing.T) {
	for _, corpus := range []string{"big-int.json", "unsorted-keys.json"} {
		t.Run(corpus, func(t *testing.T) {
			h := newHome(t)
			h.seed(t, corpus)
			before := string(h.read(t))

			if _, errOut, code := runInit(t, h); code != 0 {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			after := string(h.read(t))

			// Every key that was there is still there with byte-identical raw
			// text. Comparing the raw JSON rather than the decoded value is the
			// point: 12345678901234567 exceeds float64's exact integer range,
			// so anything that round-trips the document through a generic
			// decoder gives back 12345678901234568 and a decoded comparison
			// would not notice.
			gjson.Parse(before).ForEach(func(k, v gjson.Result) bool {
				got := gjson.Get(after, k.String())
				if !got.Exists() {
					t.Errorf("key %q was lost", k.String())
				} else if got.Raw != v.Raw {
					t.Errorf("key %q changed\nbefore: %s\n after: %s", k.String(), v.Raw, got.Raw)
				}
				return true
			})
			if strings.Contains(corpus, "big-int") && !strings.Contains(after, "12345678901234567") {
				t.Errorf("the 17-digit integer was rewritten:\n%s", after)
			}
			if strings.HasSuffix(before, "\n") && !strings.HasSuffix(after, "\n") {
				t.Error("the trailing newline was eaten")
			}

			// The added key is the only textual difference, and it is indented
			// to match the file it landed in — a settings.json people open by
			// hand should not come back looking damaged.
			if !strings.Contains(after, "\n  \"statusLine\": {\n") {
				t.Errorf("the inserted block does not match the file's own layout:\n%s", after)
			}
		})
	}
}

// TestInitRecordsPaddingWithoutClobberingIt is §9.3: "a user-set padding is
// recorded into [general] padding, not clobbered."
func TestInitRecordsPaddingWithoutClobberingIt(t *testing.T) {
	h := newHome(t)
	h.seed(t, "existing-with-padding.json")

	out, errOut, code := runInit(t, h)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	// Not clobbered: it is still in settings.json.
	if p := gjson.GetBytes(h.read(t), "statusLine.padding"); !p.Exists() || p.Int() != 0 {
		t.Errorf("padding was dropped from settings.json:\n%s", h.read(t))
	}
	// Recorded: it is now in the config too, so the width budget agrees with
	// the space Claude Code actually leaves.
	cfg, err := os.ReadFile(h.config)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(cfg), "padding") {
		t.Errorf("padding was not recorded in the config:\n%s", cfg)
	}
	if !strings.Contains(out, "padding") {
		t.Errorf("the padding decision was not explained:\n%s", out)
	}

	// And it converges: a second run is a no-op rather than an endless rewrite.
	before := h.read(t)
	if _, _, code := runInit(t, h); code != 0 {
		t.Fatalf("second init: exit %d", code)
	}
	if !bytes.Equal(before, h.read(t)) {
		t.Error("carrying padding through made init non-idempotent")
	}
}

func TestInitReplacesAnotherStatusLineAndBacksItUp(t *testing.T) {
	h := newHome(t)
	h.seed(t, "existing-statusline.json")
	before := h.read(t)

	out, errOut, code := runInit(t, h)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := gjson.GetBytes(h.read(t), "statusLine.command").String(); got != fakeExe+" render" {
		t.Errorf("command = %q", got)
	}
	backups := h.backups(t)
	if len(backups) != 1 {
		t.Fatalf("want exactly one backup, got %v", backups)
	}
	saved, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, before) {
		t.Error("the backup is not the file as it was before the edit")
	}
	if !strings.Contains(out, "replaced") {
		t.Errorf("replacing someone else's status line should say so:\n%s", out)
	}
}

// TestInitDoesNotOverwriteAnExistingConfig is §10.2 step 2: "never overwrite
// without --force". The config is where a user's evening went.
func TestInitDoesNotOverwriteAnExistingConfig(t *testing.T) {
	h := newHome(t)
	if err := os.MkdirAll(filepath.Dir(h.config), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "[general]\nicons = \"ascii\"\n# my careful notes\n"
	if err := os.WriteFile(h.config, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	got, err := os.ReadFile(h.config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != mine {
		t.Errorf("config was overwritten:\n%s", got)
	}

	if _, errOut, code := runInit(t, h, "--force"); code != 0 {
		t.Fatalf("--force: exit %d: %s", code, errOut)
	}
	got, err = os.ReadFile(h.config)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) == mine {
		t.Error("--force did not overwrite")
	}
}

// TestInitUsesTheResolvedConfig is §10.2 step 3: the desired value is computed
// from the fully resolved config, so a refresh_interval the user edited
// propagates into settings.json on a later run.
func TestInitUsesTheResolvedConfig(t *testing.T) {
	h := newHome(t)
	if err := os.MkdirAll(filepath.Dir(h.config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.config, []byte("[general]\nrefresh_interval = 15\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := gjson.GetBytes(h.read(t), "statusLine.refreshInterval").Int(); got != 15 {
		t.Errorf("refreshInterval = %d, want 15 — the user's config did not propagate", got)
	}
}

func TestInitPersistsTheDetectedIconSet(t *testing.T) {
	h := newHome(t)
	h.env["CC_STATUSLINE_NERDFONT"] = "1"

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	cfg, err := os.ReadFile(h.config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), `icons            = "nerdfont"`) {
		t.Errorf("the detected icon set was not persisted:\n%s", firstLines(string(cfg), 30))
	}
	// §10.2 step 1 is what makes the env var a one-time thing rather than
	// something the user has to export forever.
	if !strings.Contains(string(cfg), "# ") {
		t.Error("substituting into the preset stripped its comments")
	}
}

func TestInitIconsFlagOverridesDetection(t *testing.T) {
	h := newHome(t)
	h.env["CC_STATUSLINE_NERDFONT"] = "1"

	if _, errOut, code := runInit(t, h, "--icons", "ascii"); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	cfg, err := os.ReadFile(h.config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), `icons            = "ascii"`) {
		t.Errorf("--icons did not win:\n%s", firstLines(string(cfg), 30))
	}
}

// TestInitOnTheMinimalPreset covers the insertion branch: minimal.toml names
// neither icons nor powerline, so the key has to be placed under [general]
// rather than appended to the end of the file — where it would land inside
// whichever table came last.
func TestInitOnTheMinimalPreset(t *testing.T) {
	h := newHome(t)
	h.env["CC_STATUSLINE_ASCII"] = "1"

	if _, errOut, code := runInit(t, h, "--preset", "minimal"); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	raw, err := os.ReadFile(h.config)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `icons = "ascii"`) {
		t.Fatalf("icons was not inserted:\n%s", body)
	}

	// It has to be inside [general], not in some later table.
	general := strings.Index(body, "[general]")
	icons := strings.Index(body, `icons = "ascii"`)
	next := strings.Index(body[general+1:], "\n[")
	if icons < general {
		t.Error("icons was inserted before [general]")
	}
	if next >= 0 && icons > general+1+next {
		t.Errorf("icons landed in a later table:\n%s", body)
	}

	// And the whole thing still loads.
	if _, _, code := runInit(t, h); code != 0 {
		t.Error("the config written by --preset minimal does not reload cleanly")
	}
}

func TestInitDryRunTouchesNothing(t *testing.T) {
	h := newHome(t)
	h.seed(t, "plain.json")
	before := h.read(t)

	out, errOut, code := runInit(t, h, "--dry-run")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !bytes.Equal(before, h.read(t)) {
		t.Error("--dry-run modified settings.json")
	}
	if _, err := os.Stat(h.config); err == nil {
		t.Error("--dry-run wrote a config file")
	}
	if len(h.backups(t)) != 0 {
		t.Error("--dry-run made a backup")
	}
	if !strings.Contains(out, "would install") {
		t.Errorf("--dry-run should say what it would do:\n%s", out)
	}
}

// TestInitFollowsASymlinkedSettingsFile is the corpus's "symlinked" case: a
// dotfiles repository is the normal reason for it, and replacing the link with
// a regular file would detach the user's settings.json from their repository.
func TestInitFollowsASymlinkedSettingsFile(t *testing.T) {
	h := newHome(t)
	real := filepath.Join(h.dir, "dotfiles", "claude-settings.json")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(real, []byte("{\n  \"theme\": \"dark\"\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, h.settings); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	info, err := os.Lstat(h.settings)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced with a regular file")
	}
	body, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if !gjson.GetBytes(body, "statusLine").Exists() {
		t.Errorf("the link target was not patched:\n%s", body)
	}
	// The backup belongs next to the real file, not next to the link.
	if matches, _ := filepath.Glob(real + ".bak-*"); len(matches) != 1 {
		t.Errorf("backup not written beside the target: %v", matches)
	}
}

func TestInitOnAnEmptySettingsFile(t *testing.T) {
	h := newHome(t)
	h.seed(t, "empty.json")

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	raw := h.read(t)
	if !gjson.ValidBytes(raw) {
		t.Fatalf("an empty file produced invalid JSON:\n%s", raw)
	}
	if !gjson.GetBytes(raw, "statusLine").Exists() {
		t.Errorf("nothing was written:\n%s", raw)
	}
}

// TestInitWithoutAHome is the `env -i` case §9.3 requires everywhere else. It
// cannot install, and the important part is that it says so on stderr and exits
// non-zero rather than writing into a path built from an empty string.
func TestInitWithoutAHome(t *testing.T) {
	old := executable
	executable = func() (string, error) { return fakeExe, nil }
	t.Cleanup(func() { executable = old })

	var out, errb bytes.Buffer
	code := Init(nil, map[string]string{}, &out, &errb)
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "HOME") {
		t.Errorf("stderr should name the missing variable: %q", errb.String())
	}
}

func TestInitRejectsBadFlags(t *testing.T) {
	h := newHome(t)
	for _, args := range [][]string{
		{"--preset", "nonexistent"},
		{"--icons", "emoji"},
		{"--nonsense"},
	} {
		var out, errb bytes.Buffer
		if code := Init(args, h.env, &out, &errb); code != 2 {
			t.Errorf("Init(%v) = %d, want 2", args, code)
		}
		if _, err := os.Stat(h.settings); err == nil {
			t.Errorf("Init(%v) wrote settings.json despite a bad flag", args)
		}
	}
}

// TestInitCommandIsShellSafe covers the macOS `Application Support` case. Claude
// Code runs the command through a shell, so a path with a space in it has to
// come back quoted or the status line silently never starts.
func TestInitCommandIsShellSafe(t *testing.T) {
	h := newHome(t)
	spaced := "/Users/x/Library/Application Support/bin/cc-statusline"
	old := executable
	executable = func() (string, error) { return spaced, nil }
	t.Cleanup(func() { executable = old })

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	got := gjson.GetBytes(h.read(t), "statusLine.command").String()
	if want := `'` + spaced + `' render`; got != want {
		t.Errorf("command = %q, want %q", got, want)
	}
}

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/usr/local/bin/cc-statusline":   "/usr/local/bin/cc-statusline",
		"/home/u/.local/bin/cc-status":   "/home/u/.local/bin/cc-status",
		"/opt/App Support/cc-statusline": `'/opt/App Support/cc-statusline'`,
		"/opt/it's/cc-statusline":        `'/opt/it'\''s/cc-statusline'`,
		"":                               "''",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
