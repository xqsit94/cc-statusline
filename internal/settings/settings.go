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

const Key = "statusLine"

const defaultMode fs.FileMode = 0o600

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

type Desired struct {
	Type            string `json:"type"`
	Command         string `json:"command"`
	RefreshInterval int    `json:"refreshInterval"`
	Padding         *int   `json:"padding,omitempty"`
}

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
	return !hasUnknownKeys(raw)
}

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

type File struct {
	Path   string
	Target string
	Raw    []byte
	Mode   fs.FileMode
	Exists bool
}

var ErrNoPath = errors.New("cannot locate settings.json: HOME and CLAUDE_CONFIG_DIR are both unset")

func Read(path string) (*File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, ErrNoPath
	}
	f := &File{Path: path, Target: path, Mode: defaultMode}

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

func (f *File) body() []byte {
	trimmed := strings.TrimRight(string(f.Raw), " \t\r\n")
	if trimmed == "" {
		return []byte("{}")
	}
	return []byte(trimmed)
}

func (f *File) trailer() string {
	raw := string(f.Raw)
	suffix := raw[len(strings.TrimRight(raw, " \t\r\n")):]
	if suffix == "" {
		return "\n"
	}
	return suffix
}

func (f *File) Editable() bool {
	return gjson.ValidBytes(f.body())
}

func (f *File) StatusLine() (string, bool) {
	r := gjson.GetBytes(f.body(), Key)
	if !r.Exists() {
		return "", false
	}
	return r.Raw, true
}

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

func insertKey(body []byte, d Desired, indent string) ([]byte, bool) {
	s := string(body)
	if !strings.HasPrefix(strings.TrimLeft(s, " \t\r\n"), "{") {
		return nil, false
	}
	close := strings.LastIndex(s, "}")
	if close < 0 {
		return nil, false
	}

	encoded, err := json.MarshalIndent(d, indent, indent)
	if err != nil {
		return nil, false
	}

	head := strings.TrimRight(s[:close], " \t\r\n")
	sep := ",\n"
	if strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(head), "{")) == "" {
		sep = "\n"
	}
	return []byte(head + sep + indent + `"` + Key + `": ` + string(encoded) + "\n" + s[close:]), true
}

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

var ErrNotPlainJSON = errors.New("settings.json is not plain JSON (it has comments or trailing commas)")

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
	defer os.Remove(name)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, f.Mode); err != nil {
		return err
	}
	return os.Rename(name, f.Target)
}

func ManualBlock(d Desired) string {
	encoded, err := json.MarshalIndent(d, "  ", "  ")
	if err != nil {
		return fmt.Sprintf(`  "statusLine": {"type": %q, "command": %q, "refreshInterval": %d}`,
			d.Type, d.Command, d.RefreshInterval)
	}
	return `  "` + Key + `": ` + string(encoded)
}
