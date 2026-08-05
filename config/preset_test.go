package presets_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	presets "github.com/xqsit94/cc-statusline/config"
	"github.com/xqsit94/cc-statusline/internal/config"
)

// install writes a preset where config.Load will find it, which is also exactly
// what `cc-statusline init` does at M5. Testing the presets through Load rather
// than through a bare decode is the point: it is the path a user's file takes.
func install(t *testing.T, body string) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"CC_STATUSLINE_CONFIG": path}
}

// TestDefaultPresetMatchesDefaults is the guard that keeps the file users read
// and the behaviour they get from drifting apart.
//
// default.toml is documentation that happens to be executable. Every value in
// it is the embedded default, so installing it verbatim must change nothing —
// and the moment a default moves in Go without moving in the TOML, the comment
// beside it becomes a lie that nothing else would catch.
func TestDefaultPresetMatchesDefaults(t *testing.T) {
	cfg, notes := config.Load(install(t, presets.Default))

	if len(notes) != 0 {
		// A note here means the shipped file contains something the validator
		// rejects — a typo'd key, a stale colour, a placeholder that no longer
		// exists. Every one of those would ship to users silently.
		t.Fatalf("default.toml does not validate cleanly: %v", notes)
	}
	if !reflect.DeepEqual(cfg, config.Defaults()) {
		t.Error("default.toml does not decode to the embedded defaults:\n" +
			diffConfig(config.Defaults(), cfg))
	}
}

// TestMinimalPresetIsOneLine covers §7.2's claim that "the number of [[line]]
// blocks is the line count; minimal.toml ships one".
func TestMinimalPresetIsOneLine(t *testing.T) {
	cfg, notes := config.Load(install(t, presets.Minimal))
	if len(notes) != 0 {
		t.Fatalf("minimal.toml does not validate cleanly: %v", notes)
	}
	if len(cfg.Lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(cfg.Lines))
	}
	// The two segments the preset exists to keep. If either became droppable,
	// a narrow terminal could render a line that answers neither of the
	// questions the preset is built around.
	never := map[string]bool{}
	for _, ref := range cfg.Lines[0].Segments {
		if ref.Drop >= config.NeverDrop {
			never[ref.Name] = true
		}
	}
	for _, want := range []string{"context", "branch"} {
		if !never[want] {
			t.Errorf("%s is droppable in minimal.toml", want)
		}
	}
}

func TestByName(t *testing.T) {
	for _, name := range append(presets.Names(), "") {
		body, ok := presets.ByName(name)
		if !ok {
			t.Errorf("ByName(%q) = false", name)
		}
		if body == "" {
			t.Errorf("ByName(%q) is empty; the embed did not resolve", name)
		}
	}
	if _, ok := presets.ByName("nonsense"); ok {
		t.Error(`ByName("nonsense") = true`)
	}
}

// diffConfig names the top-level sections that differ. reflect.DeepEqual on a
// struct this size reports "not equal" and nothing else, which turns a one-word
// typo in a TOML file into a hunt.
func diffConfig(want, got *config.Config) string {
	w, g := reflect.ValueOf(*want), reflect.ValueOf(*got)
	var out strings.Builder
	for i := range w.NumField() {
		wf, gf := w.Field(i).Interface(), g.Field(i).Interface()
		if !reflect.DeepEqual(wf, gf) {
			fmt.Fprintf(&out, "  %s:\n    want: %+v\n     got: %+v\n",
				w.Type().Field(i).Name, wf, gf)
		}
	}
	return out.String()
}
