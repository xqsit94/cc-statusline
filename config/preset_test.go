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

func install(t *testing.T, body string) map[string]string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"CC_STATUSLINE_CONFIG": path}
}

func TestDefaultPresetMatchesDefaults(t *testing.T) {
	body, ok := presets.ByName(presets.Default)
	if !ok {
		t.Fatal("there is no default preset")
	}
	if cfg, _ := config.Load(install(t, body)); !reflect.DeepEqual(cfg, config.Defaults()) {
		t.Error("default.toml does not decode to the embedded defaults:\n" +
			diffConfig(config.Defaults(), cfg))
	}
}

func TestEveryPresetLoadsCleanly(t *testing.T) {
	names := presets.Names()
	if len(names) == 0 {
		t.Fatal("no presets are embedded at all")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			body, ok := presets.ByName(name)
			if !ok {
				t.Fatalf("%s is in Names but ByName does not resolve it", name)
			}
			if _, notes := config.Load(install(t, body)); len(notes) != 0 {
				t.Errorf("%s.toml does not validate cleanly: %v", name, notes)
			}
		})
	}
}

func TestTheDefaultPresetLeadsTheList(t *testing.T) {
	names := presets.Names()
	if len(names) == 0 || names[0] != presets.Default {
		t.Errorf("Names() = %v, want %q first", names, presets.Default)
	}
}

func TestNamesIsNotAliased(t *testing.T) {
	got := presets.Names()
	if len(got) == 0 {
		t.Fatal("no presets")
	}
	got[0] = "clobbered"
	if again := presets.Names(); again[0] == "clobbered" {
		t.Error("Names() hands out the package's own slice")
	}
}

func TestMinimalPresetIsOneLine(t *testing.T) {
	body, ok := presets.ByName("minimal")
	if !ok {
		t.Fatal("the minimal preset is not embedded")
	}
	cfg, _ := config.Load(install(t, body))
	if len(cfg.Lines) != 1 {
		t.Fatalf("got %d lines, want 1", len(cfg.Lines))
	}
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

func TestSummaryIsTheFilesOwnFirstLine(t *testing.T) {
	for _, name := range presets.Names() {
		body, _ := presets.ByName(name)
		got := presets.Summary(body)

		switch {
		case got == "":
			t.Errorf("%s: no summary; the file's first comment line did not parse", name)
		case strings.HasPrefix(got, "#"), strings.Contains(got, "cc-statusline —"):
			t.Errorf("%s: summary still carries its prefix: %q", name, got)
		case !strings.Contains(body, got):
			t.Errorf("%s: summary %q is not text from the file", name, got)
		}
	}

	if got := presets.Summary("[general]\n# not the first line\n"); got != "" {
		t.Errorf(`Summary of a comment-less file = %q, want ""`, got)
	}
}

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
