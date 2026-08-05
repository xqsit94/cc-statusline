// Package settings edits Claude Code's settings.json.
//
// # Why this is surgery and not serialisation
//
// settings.json is the user's file. It holds permissions, hooks, environment,
// MCP servers, and a theme — none of which are ours. Decoding it into a struct
// and re-encoding would silently drop every key this build does not know about,
// reorder the rest, and reformat the whole document. So the only edit performed
// here is `sjson.SetBytes(raw, "statusLine", …)`, which rewrites the bytes
// around one key and leaves the rest of the file exactly as it was found.
//
// # The comment refusal, and what C-1 actually found
//
// PRD §10.2 step 5 predicted that gjson and sjson "would mis-locate the edit"
// on a file containing comments, and asked for that to be verified before M6.
// It was, and the prediction was wrong in its reasoning and right in its
// conclusion.
//
// sjson does not mis-locate anything on an ordinary commented file: it appends
// the key before the closing brace and the comments survive untouched. The
// failure is narrower and much worse. gjson's scanner does not know that a
// comment is not data, so a file containing
//
//	// "statusLine": {"type": "command"},
//
// reports `statusLine` as *present*, and sjson rewrites the value **inside the
// comment**. The user is left with a commented-out status line that mentions
// this binary, no live `statusLine` key at all, and an `init` that reports
// success — and will keep reporting success on every later run, because the
// idempotence check reads back the value it wrote into the comment. A visibly
// corrupted file would be better than that; a silent no-op is undiagnosable.
//
// Trailing commas fail differently and just as badly: `{"a": 1,}` becomes
// `{"a": 1,,"statusLine":…}`, which is not valid under any JSON dialect.
//
// So the refusal path stays, but the gate is `gjson.ValidBytes` rather than a
// hand-written comment scanner. That is not a shortcut — it is the more correct
// instrument. §9.3 requires that `//` inside a string literal is *not* treated
// as a comment, and a scanner of ours would have to track string state, escapes,
// and escaped backslashes to get that right. gjson's parser already does, and
// it rejects the trailing-comma case §10.2 never thought to mention.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Key is the only key in the file this package will touch.
const Key = "statusLine"

// defaultMode is used when the file does not exist yet. settings.json holds no
// secrets, but it is squarely the user's own configuration, so it is created
// 0600 rather than 0644. An existing file keeps whatever mode it already has.
const defaultMode fs.FileMode = 0o600

// Path resolves the file to patch: $CLAUDE_CONFIG_DIR/settings.json if set,
// else ~/.claude/settings.json.
//
// It reads a map rather than os.Getenv for the same reason internal/style does
// (PRD §6.4), plus one specific to this package: §9.3 requires the install
// tests to run against `t.TempDir()` as HOME, and a package that reached for
// the process environment could only be tested by mutating it.
//
// An empty result means HOME is unset — an `env -i` invocation. That is not an
// error here, but it is unpatchable, and Read says so.
func Path(env map[string]string) string {
	if d := strings.TrimSpace(env["CLAUDE_CONFIG_DIR"]); d != "" {
		return filepath.Join(d, "settings.json")
	}
	home := strings.TrimSpace(env["HOME"])
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// Desired is the value §10.2 writes under `statusLine`.
//
// It is a struct rather than a map so that the three keys land in the order
// PRD §10.2 prints them. sjson marshals a map with sorted keys, which would put
// `command` before `type` and make the file disagree with the documentation for
// no reason a reader could work out.
type Desired struct {
	Type            string `json:"type"`
	Command         string `json:"command"`
	RefreshInterval int    `json:"refreshInterval"`
	// Padding is carried through when the user already set one, and is nil
	// otherwise. §10.2: padding is not written, and an existing one is not
	// clobbered — it is mirrored into `[general] padding` so that this build's
	// width budget agrees with the space Claude Code actually leaves.
	Padding *int `json:"padding,omitempty"`
}

// Equal reports whether an existing value already says what Desired says.
//
// It compares the decoded value rather than the raw bytes, because whitespace
// and key order in the user's file are not disagreements. This is what makes
// `init` idempotent: §9.3 requires a second run to produce no backup and no
// modification, and a byte comparison would fail that the moment someone
// reformatted their settings.json.
func (d Desired) Equal(raw string) bool {
	if !gjson.Valid(raw) {
		return false
	}
	var got Desired
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		return false
	}
	if got.Type != d.Type || got.Command != d.Command || got.RefreshInterval != d.RefreshInterval {
		return false
	}
	switch {
	case got.Padding == nil && d.Padding == nil:
	case got.Padding == nil || d.Padding == nil:
		return false
	case *got.Padding != *d.Padding:
		return false
	}
	// A key this build does not know about — say a future `alignment` — means
	// the value is not the one we would write, so it is not equal, and step 6
	// backs the file up before replacing it.
	return !hasUnknownKeys(raw)
}

// knownKeys is the set Desired can round-trip. It exists so that Equal can tell
// "identical" from "identical in the fields we happen to model".
var knownKeys = map[string]bool{
	"type": true, "command": true, "refreshInterval": true, "padding": true,
}

func hasUnknownKeys(raw string) bool {
	unknown := false
	gjson.Parse(raw).ForEach(func(k, _ gjson.Result) bool {
		if !knownKeys[k.String()] {
			unknown = true
			return false
		}
		return true
	})
	return unknown
}

// File is settings.json as found on disk.
type File struct {
	// Path is where we looked.
	Path string
	// Target is Path with symlinks resolved, and is what Write replaces.
	// Writing through the link would replace the link with a regular file and
	// detach a user who deliberately keeps their dotfiles in a repository.
	Target string
	// Raw is the file's bytes, or nil when it does not exist.
	Raw []byte
	// Mode is the existing file's permissions, or defaultMode when absent.
	Mode fs.FileMode
	// Exists distinguishes "no file" from "empty file"; both are patchable,
	// but only one of them is worth mentioning to the user.
	Exists bool
}

// ErrNoPath is returned when the environment gives us nowhere to look.
var ErrNoPath = errors.New("cannot locate settings.json: HOME and CLAUDE_CONFIG_DIR are both unset")

// Read loads the file. A missing file is not an error — it is the ordinary
// first-install case, and Patch starts from `{}`.
func Read(path string) (*File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrNoPath
	}
	f := &File{Path: path, Target: path, Mode: defaultMode}

	// Resolved before reading so that Target is right even if the read fails.
	// EvalSymlinks fails on a path that does not exist, which is not
	// interesting: Target stays as given and Write creates it there.
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		f.Target = resolved
	}

	raw, err := os.ReadFile(f.Target)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(f.Target); err == nil {
		f.Mode = info.Mode().Perm()
	}
	f.Raw, f.Exists = raw, true
	return f, nil
}

// body is the document to edit, with trailing whitespace removed. An absent or
// whitespace-only file becomes an empty object: there is nothing in it to
// preserve, and refusing to install because settings.json is zero bytes would
// be a strange thing to do to someone who has never opened it.
func (f *File) body() []byte {
	trimmed := strings.TrimRight(string(f.Raw), " \t\r\n")
	if trimmed == "" {
		return []byte("{}")
	}
	return []byte(trimmed)
}

// trailer is the whitespace that followed the closing brace.
//
// sjson writes up to and including `}` and drops anything after it, so without
// this every `init` would strip the newline at the end of the user's
// settings.json. That is one byte, and it is the byte that makes `git diff`
// print "\ No newline at end of file" against a file we were asked only to add
// a key to. A file with no trailing newline gets one, because we are creating
// or rewriting a text file and POSIX says text files end with a newline.
func (f *File) trailer() string {
	raw := string(f.Raw)
	suffix := raw[len(strings.TrimRight(raw, " \t\r\n")):]
	if suffix == "" {
		return "\n"
	}
	return suffix
}

// Editable reports whether the file is plain JSON, and therefore whether sjson
// can be trusted with it. See the package comment for what a false here is
// protecting against.
//
// An absent or empty file is editable — there is nothing there to be wrong.
func (f *File) Editable() bool {
	return gjson.ValidBytes(f.body())
}

// StatusLine returns the existing value's raw JSON.
//
// It is only meaningful when Editable is true. On a file with comments, gjson
// will happily return a value it found inside one, which is the entire reason
// callers must check Editable first.
func (f *File) StatusLine() (string, bool) {
	r := gjson.GetBytes(f.body(), Key)
	if !r.Exists() {
		return "", false
	}
	return r.Raw, true
}

// ExistingPadding reads a `padding` the user already set, so `init` can carry
// it through rather than dropping it.
func (f *File) ExistingPadding() (int, bool) {
	raw, ok := f.StatusLine()
	if !ok {
		return 0, false
	}
	p := gjson.Get(raw, "padding")
	if !p.Exists() || p.Type != gjson.Number {
		return 0, false
	}
	return int(p.Int()), true
}

// Patch returns the file's bytes with `statusLine` set to d.
//
// Replacing an existing key is sjson's job and it does it in place, preserving
// the surrounding layout. Adding a new one is not: sjson appends
// `,"statusLine":{…}` immediately before the closing brace and does not indent
// it, so a hand-formatted settings.json comes back with a comma at column zero
// and a doubled brace on the last line. That is valid JSON and it reads as
// damage — which is a poor thing to hand back to someone from a tool whose
// entire argument is that it does not surprise you.
//
// So the insert is done here, at the indentation the file already uses, and
// sjson is the fallback for any shape this does not recognise. Nothing else in
// the document is touched either way: §9.3 requires that non-alphabetical key
// order and a 17-digit integer survive byte-identical, and reformatting the
// file to make the diff tidy would silently rewrite the user's numbers.
func (f *File) Patch(d Desired) ([]byte, error) {
	if !f.Editable() {
		return nil, ErrNotPlainJSON
	}
	body := f.body()

	if _, exists := f.StatusLine(); !exists {
		if out, ok := insertKey(body, d, f.indent()); ok {
			return append(out, f.trailer()...), nil
		}
	}

	out, err := sjson.SetBytes(body, Key, d)
	if err != nil {
		return nil, err
	}
	return append(out, f.trailer()...), nil
}

// indent is the indentation the file already uses, taken from its first
// indented line. Two spaces is the fallback, and it is what Claude Code writes.
func (f *File) indent() string {
	for _, line := range strings.Split(string(f.Raw), "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || len(trimmed) == len(line) {
			continue
		}
		return line[:len(line)-len(trimmed)]
	}
	return "  "
}

// insertKey adds `"statusLine": {…}` as the last member of the root object.
//
// It reports false rather than guessing whenever the document is not an object
// it can place a key into — a root array, a bare scalar, anything unexpected —
// and the caller falls back to sjson, which handles those correctly if
// unattractively. The parse is safe only because Editable has already
// established the bytes are plain JSON; on a commented file the closing brace
// found below could be one inside a comment.
func insertKey(body []byte, d Desired, indent string) ([]byte, bool) {
	s := string(body)
	if !strings.HasPrefix(strings.TrimLeft(s, " \t\r\n"), "{") {
		return nil, false
	}
	close := strings.LastIndex(s, "}")
	if close < 0 {
		return nil, false
	}

	// MarshalIndent's prefix goes on every line after the first, so the value's
	// interior lands one level in from the key and its closing brace lines up
	// with the key itself.
	encoded, err := json.MarshalIndent(d, indent, indent)
	if err != nil {
		return nil, false
	}

	head := strings.TrimRight(s[:close], " \t\r\n")
	sep := ",\n"
	if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(head), "{")) == "" {
		// An empty object: `{}`. There is no previous member to comma after.
		sep = "\n"
	}
	return []byte(head + sep + indent + `"` + Key + `": ` + string(encoded) + "\n" + s[close:]), true
}

// Unpatch returns the file's bytes with `statusLine` removed. §10.3.
func (f *File) Unpatch() ([]byte, error) {
	if !f.Editable() {
		return nil, ErrNotPlainJSON
	}
	out, err := sjson.DeleteBytes(f.body(), Key)
	if err != nil {
		return nil, err
	}
	return append(out, f.trailer()...), nil
}

// ErrNotPlainJSON is the refusal. The message names the two dialects that
// trigger it because "invalid JSON" would send the user looking for a typo in a
// file that opens fine in their editor.
var ErrNotPlainJSON = errors.New("settings.json is not plain JSON (it has comments or trailing commas)")

// Backup copies the file aside before it is modified. stamp is supplied by the
// caller rather than read from the clock here, so that tests get a name they
// can predict.
//
// It returns "" when there was no file to back up.
func Backup(f *File, stamp string) (string, error) {
	if !f.Exists {
		return "", nil
	}
	path := f.Target + ".bak-" + stamp
	if err := os.WriteFile(path, f.Raw, f.Mode); err != nil {
		return "", err
	}
	return path, nil
}

// Write replaces the file's contents atomically.
//
// Temp-and-rename in the same directory, because rename is atomic only within a
// filesystem and /tmp is routinely a different one. A reader that opens
// settings.json while this runs sees either the old file or the new one, never
// a half-written object — and Claude Code reads this file on a timer.
func Write(f *File, b []byte) error {
	dir := filepath.Dir(f.Target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // no-op once the rename succeeds

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	// Closed and synced before the rename: rename orders the directory entry,
	// not the file's data, so a crash in between could otherwise publish an
	// empty settings.json.
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// CreateTemp makes 0600. An existing file's mode is restored so that
	// installing does not quietly change who can read the user's settings.
	if err := os.Chmod(name, f.Mode); err != nil {
		return err
	}
	return os.Rename(name, f.Target)
}

// ManualBlock is what the refusal prints: the exact JSON the user has to paste.
//
// It is indented two spaces so that it drops into a hand-formatted settings.json
// without further editing, and it deliberately includes the trailing comment
// about placement — the commonest way to get this wrong is to paste an object
// into the file's root rather than a key into the root object.
func ManualBlock(d Desired) string {
	encoded, err := json.MarshalIndent(d, "  ", "  ")
	if err != nil {
		// Unreachable: Desired is three strings and two ints. Returning a
		// usable string beats returning an error nobody can act on.
		return fmt.Sprintf(`  "statusLine": {"type": %q, "command": %q, "refreshInterval": %d}`,
			d.Type, d.Command, d.RefreshInterval)
	}
	return `  "` + Key + `": ` + string(encoded)
}
