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

// executable is os.Executable, indirected so that the install tests can run
// against a path they choose. PRD §10.2 requires the absolute path, and a test
// that asserted on the test binary's own name would assert nothing.
var executable = os.Executable

// Init implements `cc-statusline init` — PRD §10.2.
//
// # One correction to §10.2's step order
//
// The document reads: step 4, compare the existing `statusLine` to the desired
// value and skip ahead if they match; step 5, refuse if the file has comments.
// Those two are in the wrong order, and the consequence is the exact failure
// §10.2 step 5 exists to prevent.
//
// gjson finds keys inside comments (see internal/settings). So on a file
// containing a commented-out `statusLine` that happens to say what we would
// have written, step 4 reads the comment, concludes the status line is already
// installed, and exits reporting success — without ever reaching the refusal in
// step 5. The user has no status line, no error, and no way to tell why.
//
// The check therefore runs first here: is this file plain JSON, and only then,
// what does it say.
func Init(args []string, env map[string]string, stdout, stderr io.Writer) int {
	opt, code, ok := parseInitFlags(args, stderr)
	if !ok {
		return code
	}

	exe, err := executable()
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: cannot determine my own path: %v\n", err)
		return 1
	}
	// os.Executable can return a path with symlinks or `..` in it. §10.2 wants
	// the absolute path because Claude Code runs the command through a
	// non-interactive shell that never sources a profile, so a relative or
	// unresolved path is a status line that silently fails to start.
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}

	settingsPath := settings.Path(env)
	file, err := settings.Read(settingsPath)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	// ── The refusal, before anything is read from the file as data ─────────
	if !file.Editable() {
		writeRefusal(stdout, file, desiredFor(exe, config.Defaults(), nil))
		return 0
	}

	// A padding the user already set is carried through rather than dropped,
	// and mirrored into the config so this build's width budget agrees with the
	// space Claude Code actually leaves. §9.3: "recorded, not clobbered".
	padding, hasPadding := file.ExistingPadding()

	// ── Steps 1-2: the config file ────────────────────────────────────────
	cfgPath := config.Path(env)
	note, err := writeConfig(cfgPath, opt, env, padding, hasPadding)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	// ── Step 3: the desired value, from the *resolved* config ─────────────
	// Reloaded after writing, so that a refresh_interval the user edited into
	// an existing config.toml propagates into settings.json on a later `init`
	// rather than being pinned to whatever this binary's default happens to be.
	cfg, notes := config.Load(env)
	caps := style.Detect(env, cfg)
	desired := desiredFor(exe, cfg, ptrIf(padding, hasPadding))

	// ── Steps 4-6: the patch ──────────────────────────────────────────────
	existing, had := file.StatusLine()
	action, backup := "installed", ""
	switch {
	case had && desired.Equal(existing):
		// §9.3: a second run produces no second backup and no modification.
		// This is the branch that makes that true, and it is why Equal compares
		// the decoded value rather than the bytes.
		action = "already installed"
	case opt.dryRun:
		action = "would install"
	default:
		patched, err := file.Patch(desired)
		if err != nil {
			fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
			return 1
		}
		// Backed up before the write, not after: a backup taken afterwards is a
		// copy of the file we just replaced.
		backup, err = settings.Backup(file, time.Now().UTC().Format("20060102T150405Z"))
		if err != nil {
			fmt.Fprintf(stderr, "cc-statusline: cannot back up %s: %v\n", file.Target, err)
			return 1
		}
		if err := settings.Write(file, patched); err != nil {
			fmt.Fprintf(stderr, "cc-statusline: cannot write %s: %v\n", file.Target, err)
			return 1
		}
		if had {
			action = "replaced"
		}
	}

	// ── Step 7: say what happened, and show it ────────────────────────────
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

// desiredFor builds the value §10.2 step 3 specifies.
//
// The command is shell-quoted because Claude Code runs it through a shell, and
// `go install` on macOS can land a binary under a path with a space in it. An
// unquoted "/Users/x/Application Support/bin/cc-statusline render" would be
// parsed as two arguments and the status line would silently never start.
func desiredFor(exe string, cfg *config.Config, padding *int) settings.Desired {
	return settings.Desired{
		Type:            "command",
		Command:         shellQuote(exe) + " render",
		RefreshInterval: cfg.General.RefreshInterval,
		Padding:         padding,
	}
}

// shellSafe is the set of characters no shell treats specially.
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

// writeConfig performs §10.2 steps 1 and 2: install a preset, with the detected
// capabilities substituted in.
//
// The substitution is textual, into the preset's own lines, rather than
// marshalling a config.Config. Marshalling would produce a correct file with
// every comment stripped, and the comments are the reason default.toml exists —
// §7.1 calls it "documentation that happens to be executable".
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

	// Capabilities are resolved against the defaults, not against the config
	// being written — the file does not exist yet, or is about to be replaced.
	caps := style.Detect(env, config.Defaults())
	icons := caps.Icons.String()
	if opt.icons != "" {
		icons = opt.icons
	}
	overrides := []tomlOverride{
		{"general", "icons", quoteTOML(icons)},
	}
	// Powerline is only pinned when the environment asked for it. Left at
	// "auto" it follows the icon set, which is what §6.1 specifies and what a
	// user who later switches fonts will want.
	if caps.Powerline && icons == "nerdfont" {
		overrides = append(overrides, tomlOverride{"general", "powerline", "true"})
	}
	if hasPadding {
		overrides = append(overrides, tomlOverride{"general", "padding", fmt.Sprint(padding)})
	}

	body, applied := applyTOMLOverrides(body, overrides)

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

type tomlOverride struct {
	table string
	key   string
	value string
}

// applyTOMLOverrides rewrites `key = …` lines inside a table, preserving the
// preset's alignment and every comment around them. A key the preset does not
// mention — `minimal.toml` names neither icons nor powerline — is inserted just
// under its table header rather than appended to the end of the file, where it
// would land inside whichever table happened to be last.
func applyTOMLOverrides(body string, overrides []tomlOverride) (string, []string) {
	lines := strings.Split(body, "\n")
	var applied []string

	for _, o := range overrides {
		pattern := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(o.key) + `\s*=\s*).*$`)
		table := ""
		done := false
		for i, l := range lines {
			if t, ok := tomlTableHeader(l); ok {
				table = t
				continue
			}
			if table != o.table {
				continue
			}
			if m := pattern.FindStringSubmatch(l); m != nil {
				if lines[i] != m[1]+o.value {
					applied = append(applied, o.key+" = "+o.value)
				}
				lines[i] = m[1] + o.value
				done = true
				break
			}
		}
		if done {
			continue
		}
		for i, l := range lines {
			if t, ok := tomlTableHeader(l); ok && t == o.table {
				lines = append(lines[:i+1], append([]string{o.key + " = " + o.value}, lines[i+1:]...)...)
				applied = append(applied, o.key+" = "+o.value)
				break
			}
		}
	}
	return strings.Join(lines, "\n"), applied
}

// tomlTableHeader recognises `[general]`. It deliberately does not understand
// `[[line]]` array-of-table headers as the same thing, because a key inserted
// into one of those would join an array element rather than a table.
func tomlTableHeader(l string) (string, bool) {
	t := strings.TrimSpace(l)
	if len(t) < 3 || t[0] != '[' || t[1] == '[' || t[len(t)-1] != ']' {
		return "", false
	}
	return t[1 : len(t)-1], true
}

func quoteTOML(s string) string { return `"` + s + `"` }

// writeRefusal is §10.2 step 5's output: the exact block to paste, and why we
// are not pasting it ourselves.
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

// writePreview renders §5.1's normal state through the same path `render` uses,
// so that what `init` prints is what the status line will print.
//
// It is the fourth consumer of internal/refstate, and the one §4.2 predicted:
// `go install` leaves no repository on disk, so the payload has to be embedded
// or there is nothing to preview against until the first real session.
func writePreview(w io.Writer, cfg *config.Config, caps style.Capabilities) {
	st, ok := refstate.ByName("normal-42")
	if !ok {
		return
	}
	fmt.Fprintln(w, "\nPreview:")
	writeBlock(w, st, cfg, caps, blockOpts{colour: true})
}
