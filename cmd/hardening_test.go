package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	"github.com/xqsit94/cc-statusline/internal/refstate"
)

func hermeticEnv() map[string]string { return map[string]string{"COLUMNS": "120"} }

func mustRender(t *testing.T, env map[string]string, stdin []byte, why string) string {
	t.Helper()
	var out bytes.Buffer
	code := Render(nil, env, bytes.NewReader(stdin), &out)
	if code != 0 {
		t.Fatalf("%s: exit = %d, want 0.\nA non-zero exit blanks the status line, and the user cannot tell that from having nothing to report.", why, code)
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Fatalf("%s: stdout is empty; the status line renders blank", why)
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Errorf("%s: output does not end in a newline: %q", why, out.String())
	}
	return out.String()
}

func FuzzRender(f *testing.F) {
	f.Setenv("XDG_CACHE_HOME", f.TempDir())

	f.Add([]byte(`{}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte(``))
	f.Add([]byte(`[1,2,3]`))
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte(`{"model":{"display_name":null}}`))
	f.Add([]byte(`{"context_window":{"context_window_size":0,"total_input_tokens":1}}`))
	f.Add([]byte(`{"cost":{"total_cost_usd":1e308,"total_duration_ms":-1}}`))
	for _, st := range refstate.All() {
		f.Add(st.Payload)
	}

	env := hermeticEnv()
	f.Fuzz(func(t *testing.T, data []byte) {
		var out bytes.Buffer
		code := Render(nil, env, bytes.NewReader(data), &out)

		if code != 0 {
			t.Fatalf("exit = %d for %q", code, data)
		}
		got := out.String()
		if strings.TrimSpace(got) == "" {
			t.Fatalf("empty stdout for %q", data)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Fatalf("no trailing newline for %q: %q", data, got)
		}
		if n := strings.Count(got, "\n"); n != 1 && n != 2 {
			t.Fatalf("%d lines for %q: %q", n, data, got)
		}
		if opens, resets := strings.Count(got, "\x1b["), strings.Count(got, "\x1b[0m"); opens > 0 && opens == resets {
			t.Fatalf("every escape in %q is a reset, so something is opening none: %q", data, got)
		}
	})
}

func TestEveryOptionalFieldRemovedIndividually(t *testing.T) {
	env := hermeticEnv()
	for _, st := range refstate.All() {
		t.Run(st.Name, func(t *testing.T) {
			var root map[string]any
			if err := json.Unmarshal(st.Payload, &root); err != nil {
				t.Fatalf("fixture does not parse: %v", err)
			}
			paths := jsonPaths(root, nil)
			if len(paths) < 5 {
				t.Fatalf("only %d paths in %s; the walker is not walking", len(paths), st.Name)
			}
			for _, p := range paths {
				t.Run(strings.Join(p, "."), func(t *testing.T) {
					var fresh map[string]any
					if err := json.Unmarshal(st.Payload, &fresh); err != nil {
						t.Fatal(err)
					}
					deletePath(fresh, p)
					raw, err := json.Marshal(fresh)
					if err != nil {
						t.Fatal(err)
					}
					mustRender(t, env, raw, "with "+strings.Join(p, ".")+" removed")
				})
			}
		})
	}
}

func jsonPaths(v any, prefix []string) [][]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out [][]string
	for _, k := range keys {
		p := append(append([]string(nil), prefix...), k)
		out = append(out, p)
		out = append(out, jsonPaths(m[k], p)...)
	}
	return out
}

func deletePath(m map[string]any, path []string) {
	for _, k := range path[:len(path)-1] {
		next, ok := m[k].(map[string]any)
		if !ok {
			return
		}
		m = next
	}
	delete(m, path[len(path)-1])
}

func TestRenderSucceedsWithGitAbsentFromPath(t *testing.T) {
	repo := syntheticRepo(t, "feat/no-git-binary")

	payload := `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"current_dir":` + quote(repo) + `,"project_dir":` + quote(repo) + `},
		"cost":{"total_cost_usd":1.5,"total_duration_ms":300000,
		        "total_lines_added":10,"total_lines_removed":2},
		"context_window":{"context_window_size":200000,"total_input_tokens":50000}}`

	env := map[string]string{"COLUMNS": "120"}
	got := mustRender(t, env, []byte(payload), "git absent from PATH")

	if !strings.Contains(got, "feat/no-git-binary") {
		t.Errorf("the branch is missing with no PATH set:\n%q\n"+
			"Discovery has started shelling out to git. PRD §3.2 and §5.8 say it reads .git/HEAD.", got)
	}
}

func TestRenderSucceedsWhenCurrentDirIsGone(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "deleted-underneath-us")
	if err := os.MkdirAll(filepath.Join(gone, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}

	payload := `{"model":{"display_name":"Claude Opus 4.6"},
		"workspace":{"current_dir":` + quote(filepath.Join(gone, "src")) + `,
		             "project_dir":` + quote(gone) + `},
		"cost":{"total_cost_usd":0.5,"total_duration_ms":120000,
		        "total_lines_added":3,"total_lines_removed":1},
		"context_window":{"context_window_size":200000,"total_input_tokens":40000}}`

	got := mustRender(t, hermeticEnv(), []byte(payload), "current_dir removed")

	if !strings.Contains(got, "deleted-underneath-us") {
		t.Errorf("the project name went missing with the directory:\n%q\n"+
			"§5.3 takes it from the basename of project_dir, which does not require the path to exist.", got)
	}
}

func syntheticRepo(t *testing.T, branch string) string {
	t.Helper()
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	head := "ref: refs/heads/" + branch + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte(head), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestTheInstalledCommandActuallyRuns(t *testing.T) {
	awkward := filepath.Join(t.TempDir(), "Application Support", "it's here")
	if err := os.MkdirAll(awkward, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(awkward, "cc-statusline")
	build := exec.Command("go", "build", "-o", bin, "..")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	h := newHome(t)
	old := executable
	executable = func() (string, error) { return bin, nil }
	t.Cleanup(func() { executable = old })

	if _, errOut, code := runInit(t, h); code != 0 {
		t.Fatalf("init: exit %d: %s", code, errOut)
	}

	command := gjson.GetBytes(h.read(t), "statusLine.command").String()
	if command == "" {
		t.Fatal("init wrote no statusLine.command")
	}
	t.Logf("installed command: %s", command)

	st, ok := refstate.ByName("normal-42")
	if !ok {
		t.Fatal("normal-42 is gone")
	}
	sh := exec.Command("env", "-i", "sh", "-c", command)
	sh.Stdin = bytes.NewReader(st.Payload)
	var out, errb bytes.Buffer
	sh.Stdout, sh.Stderr = &out, &errb

	if err := sh.Run(); err != nil {
		t.Fatalf("env -i sh -c %q: %v\nstderr: %s\n\n"+
			"The command in settings.json is not runnable. Whatever it looks like in the file,\n"+
			"this is what Claude Code does with it.", command, err, errb.String())
	}
	if strings.TrimSpace(out.String()) == "" {
		t.Errorf("the installed command ran and printed nothing")
	}
	if errb.Len() != 0 {
		t.Errorf("§3.3 says nothing reaches stderr; got %q", errb.String())
	}
}

func TestExactEscapesSurviveARealPipe(t *testing.T) {
	bin := buildBinary(t)
	st, ok := refstate.ByName("normal-42")
	if !ok {
		t.Fatal("normal-42 is gone")
	}

	run := func(env []string) string {
		t.Helper()
		cmd := exec.Command(bin, "render")
		cmd.Stdin = bytes.NewReader(st.Payload)
		cmd.Env = env
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("render: %v\nstderr: %s", err, errb.String())
		}
		if errb.Len() != 0 {
			t.Errorf("stderr: %q", errb.String())
		}
		return out.String()
	}

	coloured := run([]string{"COLUMNS=120", "TERM=xterm-256color", "COLORTERM=truecolor"})

	const (
		modelMarker = "\x1b[38;2;203;166;247m"
		modelName   = "\x1b[38;2;137;220;235m"
	)
	if !strings.Contains(coloured, modelMarker+"◆\x1b[0m") {
		t.Errorf("the model marker is not painted #cba6f7 through a pipe:\n%q", coloured)
	}
	if !strings.Contains(coloured, modelName+"Claude Opus 4.6\x1b[0m") {
		t.Errorf("the model name is not painted #89dceb through a pipe:\n%q", coloured)
	}
	if strings.Contains(coloured, "\x1b[38;5;") {
		t.Errorf("COLORTERM=truecolor produced 256-colour escapes:\n%q", coloured)
	}

	plain := run([]string{"COLUMNS=120", "TERM=xterm-256color", "COLORTERM=truecolor", "NO_COLOR=1"})
	if strings.Contains(plain, "\x1b") {
		t.Errorf("NO_COLOR=1 emitted an escape:\n%q", plain)
	}
	if !strings.Contains(plain, "Claude Opus 4.6") {
		t.Errorf("NO_COLOR=1 emitted no model name, so 'no escapes' proves nothing:\n%q", plain)
	}
	if stripEscapes(coloured) != plain {
		t.Errorf("coloured and plain differ by more than colour:\n coloured: %q\n    plain: %q",
			stripEscapes(coloured), plain)
	}
}

func stripEscapes(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
