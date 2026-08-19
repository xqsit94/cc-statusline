package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	presets "github.com/xqsit94/cc-statusline/config"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/settings"
	"github.com/xqsit94/cc-statusline/internal/style"
	"github.com/xqsit94/cc-statusline/internal/wizard"
)

func Config(args []string, env map[string]string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dryRun := fs.Bool("dry-run", false, "print what would be written instead of writing it")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	path := config.Path(env)
	if path == "" {
		fmt.Fprintln(stderr, "cc-statusline: neither CC_STATUSLINE_CONFIG nor HOME is set, so there is nowhere to save")
		return 1
	}

	body, err := os.ReadFile(path)
	source := path
	if errors.Is(err, os.ErrNotExist) {
		preset, _ := presets.ByName("default")
		body, source = []byte(preset), "the default preset (no config file yet)"
	} else if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: cannot read %s: %v\n", path, err)
		return 1
	}

	cfg, notes := config.Load(env)
	st := wizard.State{
		Config:  cfg,
		Env:     env,
		Sources: previewSources(env),
		Presets: presetChoices(),
		Columns: style.Detect(env, cfg).Columns,
		Save:    saveFunc(path, string(body), *dryRun),
		Apply:   applyFunc(env, *dryRun),
	}

	if _, err := config.ReplaceLines(string(body), cfg.Lines); err != nil {
		fmt.Fprintf(stdout, `%s cannot be rewritten automatically: %v

The wizard would have to regenerate the [[line]] blocks, and doing that would
delete what is between them. Move the comment above the first [[line]] block,
or edit the file directly — every key the wizard sets is in it already.
`, source, err)
		return 1
	}

	for _, n := range notes {
		fmt.Fprintf(stderr, "note: %s\n", n)
	}

	m := wizard.New(st)
	final, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}
	if done, ok := final.(wizard.Model); ok && done.Dirty() {
		fmt.Fprintln(stdout, "quit without saving; nothing was written")
	}
	return 0
}

func saveFunc(path, body string, dryRun bool) func(wizard.Result) (string, error) {
	return func(r wizard.Result) (string, error) {
		out, err := config.ReplaceLines(body, r.Lines)
		if err != nil {
			return "", err
		}
		out, _ = config.ApplyOverrides(out, append([]config.Override{
			{Table: "general", Key: "icons", Value: config.QuoteTOML(r.Icons)},
			{Table: "general", Key: "powerline", Value: powerlineTOML(r.Powerline)},
			{Table: "general", Key: "color", Value: config.QuoteTOML(r.Colour)},
		}, formatOverrides(r.Formats)...))
		if dryRun {
			return "--dry-run: " + path + " left alone", nil
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", err
		}
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, []byte(out), 0o644); err != nil {
			return "", err
		}
		if err := os.Rename(tmp, path); err != nil {
			os.Remove(tmp)
			return "", err
		}
		body = out
		return "saved " + path, nil
	}
}

func applyFunc(env map[string]string, dryRun bool) wizard.Apply {
	path := settings.Path(env)
	if path == "" {
		return wizard.Apply{}
	}
	return wizard.Apply{
		Target: path,
		Do: func() (string, error) {
			exe, err := selfPath()
			if err != nil {
				return "", err
			}
			file, err := settings.Read(path)
			if err != nil {
				return "", err
			}
			if !file.Editable() {
				return "", fmt.Errorf("%s is not plain JSON — it has comments or a trailing "+
					"comma, and editing it automatically is not safe; run `cc-statusline init` "+
					"for the block to paste in yourself", file.Target)
			}
			cfg, _ := config.Load(env)

			_, action, backup, err := install(exe, file, cfg, dryRun)
			if err != nil {
				return "", err
			}
			if dryRun {
				return "--dry-run: " + file.Target + " left alone", nil
			}
			note := file.Target + " — " + action
			if backup != "" {
				note += ", backed up to " + filepath.Base(backup)
			}
			return note, nil
		},
	}
}

func formatOverrides(changed []config.KeyValue) []config.Override {
	var out []config.Override
	for _, kv := range changed {
		table, key, ok := config.SplitKey(kv.Key)
		if !ok {
			continue
		}
		out = append(out, config.Override{Table: table, Key: key, Value: config.QuoteTOML(kv.Value)})
	}
	return out
}

func powerlineTOML(v string) string {
	if v == "true" || v == "false" {
		return v
	}
	return config.QuoteTOML("auto")
}

func presetChoices() []wizard.Preset {
	var out []wizard.Preset
	for _, name := range presets.Names() {
		body, ok := presets.ByName(name)
		if !ok {
			continue
		}
		cfg, _ := config.Decode(body, name+" preset")
		config.Validate(cfg)
		out = append(out, wizard.Preset{
			Name: name,
			Desc: presets.Summary(body),
			Result: wizard.Result{
				Lines:     cfg.Lines,
				Icons:     cfg.General.Icons,
				Powerline: cfg.General.Powerline.String(),
				Colour:    cfg.General.Color,
			},
		})
	}
	return out
}

func previewSources(env map[string]string) []wizard.Source {
	var out []wizard.Source

	if src, ok := capturedSource(env); ok {
		out = append(out, src)
	}
	for _, st := range refstate.References() {
		p, err := payload.Parse(st.Payload)
		if err != nil {
			continue
		}
		out = append(out, wizard.Source{Name: st.Name, Desc: st.Desc, Payload: p, Git: st.Git})
	}
	return out
}

func capturedSource(env map[string]string) (wizard.Source, bool) {
	path, err := DefaultCapturePath()
	if err != nil {
		return wizard.Source{}, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return wizard.Source{}, false
	}
	var rec capture
	if err := json.Unmarshal(raw, &rec); err != nil {
		return wizard.Source{}, false
	}
	p, err := payload.Parse(rec.Payload)
	if err != nil {
		return wizard.Source{}, false
	}
	cwd, _ := os.Getwd()
	return wizard.Source{
		Name:    "captured",
		Desc:    "your last session, from " + rec.CapturedAt,
		Payload: p,
		Git:     gitinfo.Discover(env, cwd),
		Env:     rec.Env,
	}, true
}
