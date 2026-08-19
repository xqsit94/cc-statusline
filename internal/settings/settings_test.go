package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func want() Desired {
	return Desired{Type: "command", Command: "/opt/cc/cc-statusline render", RefreshInterval: 60}
}

func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolve temp dir: %v", err)
	}
	return dir
}

func load(t *testing.T, name string) *File {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read corpus %s: %v", name, err)
	}
	return &File{Path: name, Target: name, Raw: raw, Mode: 0o600, Exists: true}
}

func TestC1TheCommentRefusalIsNecessary(t *testing.T) {
	f := load(t, "commented-out-statusline.json")

	if f.Editable() {
		t.Fatal("a file with a commented-out statusLine must not be editable")
	}

	if r := gjson.GetBytes(f.Raw, Key); !r.Exists() {
		t.Skip("gjson no longer finds keys inside comments; revisit the refusal path")
	} else if !strings.Contains(r.Raw, "disabled-on-purpose") {
		t.Fatalf("expected the commented-out value, got %s", r.Raw)
	}

	if _, err := f.Patch(want()); err != ErrNotPlainJSON {
		t.Fatalf("Patch must refuse, got %v", err)
	}
}

func TestEditableSplitsTheCorpus(t *testing.T) {
	cases := map[string]bool{
		"plain.json":                    true,
		"unsorted-keys.json":            true,
		"big-int.json":                  true,
		"existing-statusline.json":      true,
		"existing-with-padding.json":    true,
		"empty.json":                    true,
		"slashes-in-string.json":        true,
		"line-comments.json":            false,
		"block-comments.json":           false,
		"commented-out-statusline.json": false,
		"trailing-comma.json":           false,
	}
	for name, editable := range cases {
		t.Run(name, func(t *testing.T) {
			if got := load(t, name).Editable(); got != editable {
				t.Errorf("Editable() = %v, want %v", got, editable)
			}
		})
	}
}

func TestSlashesInStringsAreNotComments(t *testing.T) {
	f := load(t, "slashes-in-string.json")
	out, err := f.Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if got := gjson.GetBytes(out, "docs").String(); got != "see https://example.com/// for details" {
		t.Errorf("the URL was mangled: %q", got)
	}
	if got := gjson.GetBytes(out, "note").String(); got != "a /* not a comment */ string" {
		t.Errorf("the sentence was mangled: %q", got)
	}
}

func TestEveryOtherByteSurvives(t *testing.T) {
	for _, name := range []string{"big-int.json", "unsorted-keys.json", "plain.json"} {
		t.Run(name, func(t *testing.T) {
			f := load(t, name)
			patched, err := f.Patch(want())
			if err != nil {
				t.Fatalf("Patch: %v", err)
			}

			restored := &File{Raw: patched, Exists: true}
			back, err := restored.Unpatch()
			if err != nil {
				t.Fatalf("Unpatch: %v", err)
			}
			if string(back) != string(f.Raw) {
				t.Errorf("round trip changed the file\n before: %q\n  after: %q", f.Raw, back)
			}
			if strings.Contains(name, "big-int") && !strings.Contains(string(patched), "12345678901234567") {
				t.Errorf("the 17-digit integer was rewritten:\n%s", patched)
			}
		})
	}
}

func TestPatchWritesTheDocumentedShape(t *testing.T) {
	f := load(t, "plain.json")
	out, err := f.Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	raw := gjson.GetBytes(out, Key)
	if !raw.Exists() {
		t.Fatalf("no statusLine in:\n%s", out)
	}

	var got Desired
	if err := json.Unmarshal([]byte(raw.Raw), &got); err != nil {
		t.Fatalf("statusLine is not decodable: %v", err)
	}
	if got != (Desired{Type: "command", Command: "/opt/cc/cc-statusline render", RefreshInterval: 60}) {
		t.Errorf("got %+v", got)
	}
	if i, j := strings.Index(raw.Raw, `"type"`), strings.Index(raw.Raw, `"command"`); i > j {
		t.Errorf("key order does not match the documented block: %s", raw.Raw)
	}
	if !gjson.ValidBytes(out) {
		t.Errorf("the patched file is not valid JSON:\n%s", out)
	}
}

func TestPaddingIsCarriedNotClobbered(t *testing.T) {
	f := load(t, "existing-with-padding.json")

	got, ok := f.ExistingPadding()
	if !ok || got != 0 {
		t.Fatalf("ExistingPadding() = %d, %v; want 0, true", got, ok)
	}

	d := want()
	d.Padding = &got
	out, err := f.Patch(d)
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if p := gjson.GetBytes(out, Key+".padding"); !p.Exists() || p.Int() != 0 {
		t.Errorf("padding did not survive: %s", gjson.GetBytes(out, Key).Raw)
	}

	plain, err := load(t, "plain.json").Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if gjson.GetBytes(plain, Key+".padding").Exists() {
		t.Error("padding was written where the user set none")
	}
}

func TestEqualIsIdempotence(t *testing.T) {
	d := want()
	cases := []struct {
		name string
		raw  string
		eq   bool
	}{
		{"exact", `{"type":"command","command":"/opt/cc/cc-statusline render","refreshInterval":60}`, true},
		{"reordered and spaced", "{\n \"refreshInterval\" : 60,\n \"command\": \"/opt/cc/cc-statusline render\",\n \"type\":\"command\"\n}", true},
		{"different command", `{"type":"command","command":"/usr/bin/other","refreshInterval":60}`, false},
		{"different interval", `{"type":"command","command":"/opt/cc/cc-statusline render","refreshInterval":5}`, false},
		{"extra padding", `{"type":"command","command":"/opt/cc/cc-statusline render","refreshInterval":60,"padding":0}`, false},
		{"unknown key", `{"type":"command","command":"/opt/cc/cc-statusline render","refreshInterval":60,"alignment":"left"}`, false},
		{"not an object", `"cc-statusline render"`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := d.Equal(c.raw); got != c.eq {
				t.Errorf("Equal(%s) = %v, want %v", c.raw, got, c.eq)
			}
		})
	}
}

func TestEqualWithPadding(t *testing.T) {
	zero := 0
	d := want()
	d.Padding = &zero
	raw := `{"type":"command","command":"/opt/cc/cc-statusline render","refreshInterval":60,"padding":0}`
	if !d.Equal(raw) {
		t.Error("a carried padding must compare equal, or init rewrites the file forever")
	}
}

func TestUnpatchLeavesEverythingElse(t *testing.T) {
	f := load(t, "existing-statusline.json")
	out, err := f.Unpatch()
	if err != nil {
		t.Fatalf("Unpatch: %v", err)
	}
	if gjson.GetBytes(out, Key).Exists() {
		t.Errorf("statusLine survived:\n%s", out)
	}
	if got := gjson.GetBytes(out, "theme").String(); got != "dark" {
		t.Errorf("theme was lost: %q", got)
	}
}

func TestPathResolution(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"home", map[string]string{"HOME": "/home/u"}, "/home/u/.claude/settings.json"},
		{"config dir wins", map[string]string{"HOME": "/home/u", "CLAUDE_CONFIG_DIR": "/cfg"}, "/cfg/settings.json"},
		{"blank config dir is ignored", map[string]string{"HOME": "/home/u", "CLAUDE_CONFIG_DIR": "  "}, "/home/u/.claude/settings.json"},
		{"env -i", map[string]string{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Path(c.env); got != c.want {
				t.Errorf("Path() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestReadReportsAnUnlocatableFile(t *testing.T) {
	if _, err := Read(""); err != ErrNoPath {
		t.Errorf("Read(\"\") = %v, want ErrNoPath", err)
	}
}

func TestReadAbsentFileIsNotAnError(t *testing.T) {
	path := filepath.Join(tempDir(t), ".claude", "settings.json")
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if f.Exists {
		t.Error("Exists should be false")
	}
	if !f.Editable() {
		t.Error("an absent file must be editable — it is the first-install case")
	}
	out, err := f.Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if !gjson.ValidBytes(out) || !gjson.GetBytes(out, Key).Exists() {
		t.Errorf("patching an absent file should produce a one-key object, got %s", out)
	}
}

func TestWriteFollowsTheSymlink(t *testing.T) {
	dir := tempDir(t)
	real := filepath.Join(dir, "real-settings.json")
	link := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(real, []byte(`{"theme":"dark"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	f, err := Read(link)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if f.Target != real {
		t.Fatalf("Target = %q, want %q", f.Target, real)
	}
	out, err := f.Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if err := Write(f, out); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Lstat(link)
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
	if !gjson.GetBytes(body, Key).Exists() {
		t.Errorf("the target file was not updated:\n%s", body)
	}
}

func TestWritePreservesMode(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	out, err := f.Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if err := Write(f, out); err != nil {
		t.Fatalf("Write: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Errorf("mode = %o, want 640 — installing must not change who can read the file", got)
	}
}

func TestWriteOverAReadOnlyFile(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"theme":"dark"}`), 0o400); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	out, err := f.Patch(want())
	if err != nil {
		t.Fatalf("Patch: %v", err)
	}
	if err := Write(f, out); err != nil {
		t.Fatalf("Write over a read-only file in a writable directory: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o400 {
		t.Errorf("mode = %o, want 400", got)
	}
}

func TestWriteFailsOnAReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	dir := tempDir(t)
	sub := filepath.Join(dir, "locked")
	if err := os.Mkdir(sub, 0o500); err != nil {
		t.Fatal(err)
	}
	f := &File{Path: filepath.Join(sub, "settings.json"), Target: filepath.Join(sub, "settings.json"), Mode: 0o600}
	if err := Write(f, []byte(`{}`)); err == nil {
		t.Error("writing into a read-only directory should fail loudly, not silently succeed")
	}
}

func TestBackupCopiesTheOriginal(t *testing.T) {
	dir := tempDir(t)
	path := filepath.Join(dir, "settings.json")
	original := []byte("{\n  \"theme\": \"dark\"\n}\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	name, err := Backup(f, "20260805T120000Z")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if want := path + ".bak-20260805T120000Z"; name != want {
		t.Errorf("backup path = %q, want %q", name, want)
	}
	body, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(original) {
		t.Errorf("backup is not the original: %q", body)
	}
}

func TestBackupOfAnAbsentFileIsNothing(t *testing.T) {
	f := &File{Path: "/nonexistent/settings.json", Target: "/nonexistent/settings.json"}
	name, err := Backup(f, "stamp")
	if err != nil {
		t.Fatalf("Backup: %v", err)
	}
	if name != "" {
		t.Errorf("backed up a file that does not exist: %q", name)
	}
}

func TestManualBlockPastesCleanly(t *testing.T) {
	block := ManualBlock(want())
	if !strings.HasPrefix(block, `  "statusLine": `) {
		t.Errorf("block does not start with the key: %q", block)
	}
	wrapped := "{\n" + block + "\n}"
	if !gjson.Valid(wrapped) {
		t.Errorf("the block does not paste into an object:\n%s", wrapped)
	}
	if got := gjson.Get(wrapped, Key+".command").String(); got != want().Command {
		t.Errorf("command = %q", got)
	}
}
