package cmd

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	presets "github.com/xqsit94/cc-statusline/config"
	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/settings"
	"github.com/xqsit94/cc-statusline/internal/style"
)

var executable = os.Executable

func Init(args []string, env map[string]string, stdout, stderr io.Writer) int {
	opt, code, ok := parseInitFlags(args, stderr)
	if !ok {
		return code
	}

	exe, err := selfPath()
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	settingsPath := settings.Path(env)
	file, err := settings.Read(settingsPath)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	if !file.Editable() {
		writeRefusal(stdout, file, desiredFor(exe, config.Defaults(), nil))
		return 0
	}

	padding, hasPadding := file.ExistingPadding()

	cfgPath := config.Path(env)
	note, err := writeConfig(cfgPath, opt, env, padding, hasPadding)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	cfg, notes := config.Load(env)
	caps := style.Detect(env, cfg)

	desired, action, backup, err := install(exe, file, cfg, opt.dryRun)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "settings   %s (%s)\n", file.Target, action)
	if backup != "" {
		fmt.Fprintf(stdout, "backup     %s\n", backup)
	}
	if cfgPath == "" {
		fmt.Fprintln(stdout, "config     not written: neither CC_STATUSLINE_CONFIG nor HOME is set")
	} else {
		fmt.Fprintf(stdout, "config     %s (%s)\n", cfgPath, note)
	}
	fmt.Fprintf(stdout, "command    %s\n", desired.Command)
	fmt.Fprintf(stdout, "refresh    %ds\n", desired.RefreshInterval)
	if hasPadding {
		fmt.Fprintf(stdout, "padding    %d — kept in settings.json, mirrored into [general] padding\n", padding)
	}
	for _, n := range notes {
		fmt.Fprintf(stdout, "note       %s\n", n)
	}

	writePreview(stdout, cfg, caps)
	fmt.Fprintf(stdout, "\nRemove it with: %s uninstall\n", exe)
	return 0
}

type initOptions struct {
	preset string
	icons  string
	force  bool
	dryRun bool
}

func parseInitFlags(args []string, stderr io.Writer) (initOptions, int, bool) {
	var opt initOptions
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opt.preset, "preset", "default", "which shipped preset to install: "+strings.Join(presets.Names(), " | "))
	fs.StringVar(&opt.icons, "icons", "", "override the detected icon set: ascii | unicode | nerdfont")
	fs.BoolVar(&opt.force, "force", false, "overwrite an existing config.toml")
	fs.BoolVar(&opt.dryRun, "dry-run", false, "print what would change without touching either file")
	if err := fs.Parse(args); err != nil {
		return opt, 2, false
	}
	if _, ok := presets.ByName(opt.preset); !ok {
		fmt.Fprintf(stderr, "cc-statusline: unknown preset %q; have %s\n", opt.preset, strings.Join(presets.Names(), ", "))
		return opt, 2, false
	}
	switch opt.icons {
	case "", "ascii", "unicode", "nerdfont":
	default:
		fmt.Fprintf(stderr, "cc-statusline: unknown icon set %q; have ascii, unicode, nerdfont\n", opt.icons)
		return opt, 2, false
	}
	return opt, 0, true
}

func selfPath() (string, error) {
	exe, err := executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine my own path: %w", err)
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return exe, nil
}

func install(exe string, file *settings.File, cfg *config.Config, dryRun bool) (
	desired settings.Desired, action, backup string, err error) {

	padding, hasPadding := file.ExistingPadding()
	desired = desiredFor(exe, cfg, ptrIf(padding, hasPadding))

	existing, had := file.StatusLine()
	switch {
	case had && desired.Equal(existing):
		return desired, "already installed", "", nil
	case dryRun:
		return desired, "would install", "", nil
	}

	patched, err := file.Patch(desired)
	if err != nil {
		return desired, "", "", err
	}
	backup, err = settings.Backup(file, time.Now().UTC().Format("20060102T150405Z"))
	if err != nil {
		return desired, "", "", fmt.Errorf("cannot back up %s: %w", file.Target, err)
	}
	if err := settings.Write(file, patched); err != nil {
		return desired, "", "", fmt.Errorf("cannot write %s: %w", file.Target, err)
	}
	if had {
		return desired, "replaced", backup, nil
	}
	return desired, "installed", backup, nil
}

func desiredFor(exe string, cfg *config.Config, padding *int) settings.Desired {
	return settings.Desired{
		Type:            "command",
		Command:         shellQuote(exe) + " render",
		RefreshInterval: cfg.General.RefreshInterval,
		Padding:         padding,
	}
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellQuote(s string) string {
	if s != "" && shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func ptrIf(v int, ok bool) *int {
	if !ok {
		return nil
	}
	return &v
}

func writeConfig(path string, opt initOptions, env map[string]string, padding int, hasPadding bool) (note string, err error) {
	if path == "" {
		return "", nil
	}
	if _, statErr := os.Stat(path); statErr == nil && !opt.force {
		return "left alone; --force overwrites", nil
	}

	body, ok := presets.ByName(opt.preset)
	if !ok {
		return "", fmt.Errorf("unknown preset %q", opt.preset)
	}

	caps := style.Detect(env, config.Defaults())
	icons := caps.Icons.String()
	if opt.icons != "" {
		icons = opt.icons
	}
	overrides := []config.Override{
		{Table: "general", Key: "icons", Value: config.QuoteTOML(icons)},
	}
	if caps.Powerline && icons == "nerdfont" {
		overrides = append(overrides, config.Override{Table: "general", Key: "powerline", Value: "true"})
	}
	if hasPadding {
		overrides = append(overrides, config.Override{Table: "general", Key: "padding", Value: fmt.Sprint(padding)})
	}

	body, applied := config.ApplyOverrides(body, overrides)

	if opt.dryRun {
		return "would be written from the " + opt.preset + " preset", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return "", err
	}
	if len(applied) == 0 {
		return "written from the " + opt.preset + " preset", nil
	}
	return "written from the " + opt.preset + " preset, " + strings.Join(applied, ", "), nil
}

func writeRefusal(w io.Writer, file *settings.File, d settings.Desired) {
	fmt.Fprintf(w, `%s is not plain JSON — it contains comments or a trailing comma.

Editing it automatically is not safe. A JSON scanner cannot tell a key inside a
comment from a real one, so an automatic edit can land inside the comment and
leave you with no status line and no error message.

Add this to the top-level object yourself:

%s

Then run `+"`cc-statusline init`"+` again to confirm it took.
`, file.Target, settings.ManualBlock(d))
}

func writePreview(w io.Writer, cfg *config.Config, caps style.Capabilities) {
	st, ok := refstate.ByName("normal-42")
	if !ok {
		return
	}
	fmt.Fprintln(w, "\nPreview:")
	writeBlock(w, st, cfg, caps, blockOpts{colour: true})
}
