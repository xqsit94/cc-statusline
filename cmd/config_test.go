package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tidwall/gjson"
	presets "github.com/xqsit94/cc-statusline/config"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/wizard"
)

func configHarness(t *testing.T) (cfgPath string, env map[string]string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, "config.toml")
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))
	return cfgPath, map[string]string{
		"CC_STATUSLINE_CONFIG": cfgPath,
		"TERM":                 "xterm-256color",
		"COLUMNS":              "120",
	}
}

func TestSaveRoundTripsThroughTheLoader(t *testing.T) {
	cfgPath, env := configHarness(t)
	body, _ := presets.ByName("default")
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, _ := config.Load(env)
	var flat []config.SegmentRef
	for _, r := range loaded.Lines {
		flat = append(flat, r.Segments...)
	}
	for i, j := 0, len(flat)-1; i < j; i, j = i+1, j-1 {
		flat[i], flat[j] = flat[j], flat[i]
	}
	flat[0].Drop = 37

	want := wizard.Result{
		Lines:     []config.Line{{Segments: flat}},
		Icons:     "nerdfont",
		Powerline: "true",
		Colour:    "256",
	}

	note, err := saveFunc(cfgPath, body, false)(want)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !strings.Contains(note, cfgPath) {
		t.Errorf("the note does not say where it wrote: %q", note)
	}

	got, notes := config.Load(env)
	if len(notes) != 0 {
		t.Errorf("the saved file does not load cleanly: %v", notes)
	}
	if len(got.Lines) != 1 {
		t.Fatalf("loaded %d rows, want 1", len(got.Lines))
	}
	for i, s := range want.Lines[0].Segments {
		if got.Lines[0].Segments[i] != s {
			t.Errorf("segment %d is %+v, want %+v", i, got.Lines[0].Segments[i], s)
		}
	}
	if got.General.Icons != "nerdfont" || got.General.Color != "256" {
		t.Errorf("capability keys came back as icons=%q color=%q", got.General.Icons, got.General.Color)
	}
	if got.General.Powerline.String() != "true" {
		t.Errorf("powerline came back as %q", got.General.Powerline.String())
	}

	after, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, comment := range []string{
		"# cc-statusline — the commented reference configuration.",
		"# NO_COLOR always wins, whatever is set here.",
		"# Colours are #rrggbb only.",
	} {
		if !strings.Contains(string(after), comment) {
			t.Errorf("a comment was lost: %q", comment)
		}
	}
}

func TestASecondSaveDoesNotRevertTheFirst(t *testing.T) {
	cfgPath, env := configHarness(t)
	body, _ := presets.ByName("default")
	save := saveFunc(cfgPath, body, false)

	first := wizard.Result{
		Lines: []config.Line{{Segments: []config.SegmentRef{{Name: "model", Drop: 99}}}},
		Icons: "ascii", Powerline: "false", Colour: "none",
	}
	if _, err := save(first); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Icons = "nerdfont"
	if _, err := save(second); err != nil {
		t.Fatal(err)
	}

	got, _ := config.Load(env)
	if got.General.Icons != "nerdfont" {
		t.Errorf("icons = %q after two saves", got.General.Icons)
	}
	if len(got.Lines) != 1 || len(got.Lines[0].Segments) != 1 {
		t.Errorf("the second save reverted the first: %+v", got.Lines)
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	cfgPath, _ := configHarness(t)
	body, _ := presets.ByName("default")

	note, err := saveFunc(cfgPath, body, true)(wizard.Result{
		Lines: []config.Line{{Segments: []config.SegmentRef{{Name: "model", Drop: 99}}}},
		Icons: "ascii", Powerline: "auto", Colour: "auto",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(note, "dry-run") {
		t.Errorf("the note does not say it wrote nothing: %q", note)
	}
	if _, err := os.Stat(cfgPath); !os.IsNotExist(err) {
		t.Error("--dry-run created the file")
	}
}

func TestConfigRefusesBeforeOpeningTheWizard(t *testing.T) {
	cfgPath, env := configHarness(t)
	if err := os.WriteFile(cfgPath, []byte(`[general]
icons = "unicode"

[[line]]
segments = [{name="model", drop=99}]

# I keep this one separate on purpose
[[line]]
segments = [{name="branch", drop=99}]
`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	if code := Config(nil, env, &out, &errOut); code != 1 {
		t.Fatalf("exit %d, want 1\n%s%s", code, out.String(), errOut.String())
	}
	for _, want := range []string{"[[line]]", "delete what is between them", cfgPath} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the refusal does not mention %q:\n%s", want, out.String())
		}
	}

	after, _ := os.ReadFile(cfgPath)
	if !strings.Contains(string(after), "# I keep this one separate on purpose") {
		t.Error("the refusal path touched the file")
	}
}

func TestConfigNeedsSomewhereToSave(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := Config(nil, map[string]string{}, &out, &errOut); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "nowhere to save") {
		t.Errorf("said nothing useful: %q", errOut.String())
	}
}

func TestPreviewSourcesAlwaysHasTheFixtures(t *testing.T) {
	_, env := configHarness(t)

	got := previewSources(env)
	if len(got) < 4 {
		t.Fatalf("got %d sources with no capture on disk, want the four reference states", len(got))
	}
	for _, s := range got {
		if s.Payload == nil {
			t.Errorf("source %q carries no payload", s.Name)
		}
	}
}

func TestPresetChoicesAreTheShippedFiles(t *testing.T) {
	got := presetChoices()
	if len(got) != len(presets.Names()) {
		t.Fatalf("got %d presets, want the %d that ship", len(got), len(presets.Names()))
	}

	shapes := map[string]int{}
	for _, p := range got {
		if len(p.Lines) == 0 {
			t.Errorf("%s decoded to no rows at all", p.Name)
		}
		if p.Desc == "" {
			t.Errorf("%s has no description; the picker would show a blank line", p.Name)
		}
		for _, row := range p.Lines {
			for _, s := range row.Segments {
				if s.Drop < 0 || s.Drop > config.NeverDrop {
					t.Errorf("%s: %s has drop %d, outside the schema — Validate did not run",
						p.Name, s.Name, s.Drop)
				}
			}
		}
		shapes[p.Name] = len(p.Lines)
	}

	if shapes["default"] == shapes["minimal"] {
		t.Errorf("both presets are %d rows; the picker has nothing to distinguish",
			shapes["default"])
	}
	if shapes["minimal"] != 1 {
		t.Errorf("minimal decoded to %d rows, want §7.2's one", shapes["minimal"])
	}
}

func TestPowerlineIsWrittenAsTheSchemaSpellsIt(t *testing.T) {
	for in, want := range map[string]string{
		"true": "true", "false": "false", "auto": `"auto"`, "": `"auto"`,
	} {
		if got := powerlineTOML(in); got != want {
			t.Errorf("powerlineTOML(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestACycledFormatReachesTheFile(t *testing.T) {
	for _, preset := range []string{"default", "minimal"} {
		t.Run(preset, func(t *testing.T) {
			cfgPath, env := configHarness(t)
			body, _ := presets.ByName(preset)
			if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			loaded, _ := config.Load(env)

			want := config.Segments{}
			config.ApplyVariant(config.Variants["ratelimit_5h"][3], &want)

			if _, err := saveFunc(cfgPath, body, false)(wizard.Result{
				Lines:     loaded.Lines,
				Icons:     loaded.General.Icons,
				Powerline: loaded.General.Powerline.String(),
				Colour:    loaded.General.Color,
				Formats:   config.Changed(merge(loaded.Segments, want), loaded.Segments),
			}); err != nil {
				t.Fatalf("save: %v", err)
			}

			got, notes := config.Load(env)
			if len(notes) != 0 {
				t.Fatalf("the saved file does not load cleanly: %v", notes)
			}
			if got.Segments.RateLimit5h != want.RateLimit5h {
				t.Errorf("read back %+v, want %+v", got.Segments.RateLimit5h, want.RateLimit5h)
			}
			if got.Segments.Cost != loaded.Segments.Cost {
				t.Errorf("cost changed to %+v", got.Segments.Cost)
			}
			if len(got.Lines) != len(loaded.Lines) {
				t.Errorf("the rows changed: %d, want %d", len(got.Lines), len(loaded.Lines))
			}
		})
	}
}

func merge(base, overlay config.Segments) config.Segments {
	base.RateLimit5h = overlay.RateLimit5h
	return base
}

func TestApplyInstallsExactlyWhatInitInstalls(t *testing.T) {
	setup := func(h *home) {
		h.seed(t, "existing-with-padding.json")
		body, _ := presets.ByName("default")
		body, _ = config.ApplyOverrides(body, []config.Override{
			{Table: "general", Key: "refresh_interval", Value: "45"},
		})
		if err := os.MkdirAll(filepath.Dir(h.config), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(h.config, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a := newHome(t)
	setup(a)
	if out, errOut, code := runInit(t, a); code != 0 {
		t.Fatalf("init: exit %d\n%s%s", code, out, errOut)
	}
	viaInit := a.read(t)

	b := newHome(t)
	setup(b)
	note, err := applyFunc(b.env, false).Do()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(note, "replaced") || !strings.Contains(note, b.settings) {
		t.Errorf("the note does not say what happened or where: %q", note)
	}
	viaCtrlS := b.read(t)

	if !bytes.Equal(viaInit, viaCtrlS) {
		t.Errorf("ctrl+s installs something `init` does not:\n  init: %s\nctrl+s: %s",
			viaInit, viaCtrlS)
	}
	if got := gjson.GetBytes(viaCtrlS, "statusLine.refreshInterval").Int(); got != 45 {
		t.Errorf("refreshInterval is %d, want the config's 45:\n%s", got, viaCtrlS)
	}
	if got := gjson.GetBytes(viaCtrlS, "statusLine.padding"); !got.Exists() || got.Int() != 0 {
		t.Errorf("the user's padding was clobbered:\n%s", viaCtrlS)
	}
}

func TestApplyIsIdempotentAndBacksUpWhatItReplaces(t *testing.T) {
	h := newHome(t)
	h.seed(t, "existing-statusline.json")

	note, err := applyFunc(h.env, false).Do()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(note, "replaced") {
		t.Errorf("it replaced another status line without saying so: %q", note)
	}
	if got := h.backups(t); len(got) != 1 {
		t.Fatalf("took %d backups replacing an existing key, want 1", len(got))
	}
	after := h.read(t)

	note, err = applyFunc(h.env, false).Do()
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if !strings.Contains(note, "already installed") {
		t.Errorf("the second apply reported %q", note)
	}
	if got := h.backups(t); len(got) != 1 {
		t.Errorf("the second apply left %d backups, want the first one only", len(got))
	}
	if !bytes.Equal(after, h.read(t)) {
		t.Error("the second apply modified the file")
	}
}

func TestApplyRefusesTheFilesInitRefuses(t *testing.T) {
	agreeing := []byte(`{
  "theme": "dark"
  // "statusLine": {"type": "command", "command": "` + fakeExe + ` render", "refreshInterval": 60}
}
`)

	cases := map[string][]byte{
		"commented-out-statusline.json": nil,
		"trailing-comma.json":           nil,
		"comment-agrees-with-us":        agreeing,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHome(t)
			if raw == nil {
				h.seed(t, name)
			} else if err := os.WriteFile(h.settings, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			before := h.read(t)

			note, err := applyFunc(h.env, false).Do()
			if err == nil {
				t.Fatalf("ctrl+s did not refuse a file `init` refuses; it reported %q", note)
			}
			if !strings.Contains(err.Error(), "plain JSON") {
				t.Errorf("the refusal does not say why: %v", err)
			}
			if !bytes.Equal(before, h.read(t)) {
				t.Error("the refusal path wrote to the file anyway")
			}
			if got := h.backups(t); len(got) != 0 {
				t.Errorf("it backed up a file it did not edit: %v", got)
			}
		})
	}
}

func TestApplyDryRunTouchesNothing(t *testing.T) {
	h := newHome(t)

	note, err := applyFunc(h.env, true).Do()
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(note, "dry-run") {
		t.Errorf("the note does not say it wrote nothing: %q", note)
	}
	if _, err := os.Stat(h.settings); !os.IsNotExist(err) {
		t.Error("--dry-run created settings.json")
	}
	if got := h.backups(t); len(got) != 0 {
		t.Errorf("--dry-run left backups: %v", got)
	}
}

func TestApplyIsUnavailableWithNowhereToInstall(t *testing.T) {
	if got := applyFunc(map[string]string{}, false); got.Do != nil {
		t.Error("applying is offered with neither HOME nor CLAUDE_CONFIG_DIR set")
	}
	if got := applyFunc(map[string]string{"CLAUDE_CONFIG_DIR": "/tmp/x"}, false); got.Do == nil {
		t.Error("CLAUDE_CONFIG_DIR did not make applying available")
	} else if got.Target != filepath.Join("/tmp/x", "settings.json") {
		t.Errorf("the confirmation would name %q", got.Target)
	}
}
