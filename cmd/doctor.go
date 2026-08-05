package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/refstate"
	"github.com/xqsit94/cc-statusline/internal/settings"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// Doctor implements `cc-statusline doctor` — PRD §4.1.
//
// It answers the question a bug report cannot: what did this build actually
// see. Four things, in the order they go wrong:
//
//	install       is the status line wired up, and to this binary
//	config        which file was read, and what it could not use
//	capabilities  what was detected, and which variable decided it
//	payload       what Claude Code last sent, and how it differs from §3.1
//
// **It exits 0 when git is absent** (§4.1), and 0 when there is no payload, and
// 0 when nothing is installed. Those are all things a user can be told about
// and then fix. Exit 1 is reserved for a config file that could not be parsed
// at all — the one state where what `doctor` reports and what `render` does
// might genuinely diverge.
func Doctor(args []string, env map[string]string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	asJSON := fs.Bool("json", false, "emit a machine-readable report")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rep := diagnose(env)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rep); err != nil {
			fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
			return 1
		}
	} else {
		rep.writeText(stdout, env)
	}
	if rep.Config.Unreadable {
		return 1
	}
	return 0
}

// report is the whole of `doctor --json`. The field names are the contract for
// anyone scripting against it, so they are spelled out rather than inherited
// from Go identifiers.
type report struct {
	Version      string           `json:"version"`
	Install      installReport    `json:"install"`
	Config       configReport     `json:"config"`
	Capabilities capabilityReport `json:"capabilities"`
	Payload      payloadReport    `json:"payload"`
	LastError    string           `json:"last_error,omitempty"`
}

type installReport struct {
	SettingsPath string `json:"settings_path"`
	Exists       bool   `json:"exists"`
	PlainJSON    bool   `json:"plain_json"`
	StatusLine   string `json:"status_line,omitempty"`
	IsUs         bool   `json:"is_us"`
	Problem      string `json:"problem,omitempty"`
}

type configReport struct {
	Path       string   `json:"path"`
	Exists     bool     `json:"exists"`
	Unreadable bool     `json:"unreadable"`
	Notes      []string `json:"notes,omitempty"`
}

type capabilityReport struct {
	Icons     string `json:"icons"`
	Powerline bool   `json:"powerline"`
	Profile   string `json:"color_profile"`
	Ambiguous int    `json:"ambiguous_width"`
	Columns   int    `json:"columns"`
	Available int    `json:"available"`
	// Because is the environment that decided the above. §6.1's contract is
	// four variables and a locale, and "why is it ASCII" is answerable only if
	// they are printed next to the answer.
	Because map[string]string `json:"environment"`
}

type payloadReport struct {
	Path     string   `json:"path,omitempty"`
	Age      string   `json:"age,omitempty"`
	Decoded  bool     `json:"decoded"`
	Unknown  []string `json:"unknown_keys,omitempty"`
	Missing  []string `json:"missing_keys,omitempty"`
	Problem  string   `json:"problem,omitempty"`
	Rendered []string `json:"rendered,omitempty"`
}

func diagnose(env map[string]string) *report {
	cfg, notes := config.Load(env)
	caps := style.Detect(env, cfg)

	rep := &report{
		Version:      version,
		Install:      diagnoseInstall(env),
		Config:       diagnoseConfig(env, notes),
		Capabilities: diagnoseCapabilities(env, cfg, caps),
		Payload:      diagnosePayload(cfg, caps),
	}
	if path, err := lastErrorPath(); err == nil {
		if raw, err := os.ReadFile(path); err == nil {
			rep.LastError = strings.TrimRight(string(raw), "\n")
		}
	}
	return rep
}

func diagnoseInstall(env map[string]string) installReport {
	path := settings.Path(env)
	r := installReport{SettingsPath: path}
	if path == "" {
		r.Problem = "neither HOME nor CLAUDE_CONFIG_DIR is set, so settings.json cannot be located"
		return r
	}
	file, err := settings.Read(path)
	if err != nil {
		r.Problem = err.Error()
		return r
	}
	r.SettingsPath, r.Exists = file.Target, file.Exists
	r.PlainJSON = file.Editable()
	if !r.PlainJSON {
		r.Problem = "not plain JSON (comments or a trailing comma); init will decline to edit it"
		return r
	}
	raw, had := file.StatusLine()
	if !had {
		r.Problem = "no statusLine key; run `cc-statusline init`"
		return r
	}
	r.StatusLine = commandOf(raw)
	r.IsUs = mentionsThisBinary(raw)
	if !r.IsUs {
		r.Problem = "the statusLine key runs something else"
	}
	return r
}

func diagnoseConfig(env map[string]string, notes []config.Defaulted) configReport {
	r := configReport{Path: config.Path(env)}
	if r.Path != "" {
		if _, err := os.Stat(r.Path); err == nil {
			r.Exists = true
		}
	}
	for _, n := range notes {
		r.Notes = append(r.Notes, n.String())
		// §7.1 distinguishes a file that could not be read or parsed from keys
		// that were repaired. Only the former is an exit-1 condition: it is the
		// one case where the user's file is doing nothing at all.
		if n.Key == "config file" {
			r.Unreadable = true
		}
	}
	return r
}

func diagnoseCapabilities(env map[string]string, cfg *config.Config, caps style.Capabilities) capabilityReport {
	// Rendered against a reference state purely to get the budget, which is a
	// function of columns, padding, and width_reserve rather than of content.
	avail := 0
	if st, ok := refstate.ByName("normal-42"); ok {
		p, _ := payload.Parse(st.Payload)
		avail = line.Available(line.Context{
			Payload: p, Config: cfg, Git: st.Git, Style: style.NewStyle(caps, cfg),
		})
	}

	because := map[string]string{}
	for _, k := range []string{
		"CC_STATUSLINE_ASCII", "CC_STATUSLINE_NERDFONT", "CC_STATUSLINE_POWERLINE",
		"CC_STATUSLINE_COLOR", "CC_STATUSLINE_NO_GIT", "CC_STATUSLINE_CONFIG",
		"NO_COLOR", "COLORTERM", "TERM", "COLUMNS", "LANG", "LC_ALL", "LC_CTYPE",
	} {
		if v, ok := env[k]; ok && v != "" {
			because[k] = v
		}
	}

	return capabilityReport{
		Icons:     caps.Icons.String(),
		Powerline: caps.Powerline,
		Profile:   profileName(caps.Profile),
		Ambiguous: caps.Ambiguous,
		Columns:   caps.Columns,
		Available: avail,
		Because:   because,
	}
}

// diagnosePayload reads the last captured payload and compares its keys against
// the struct this build knows about — §3.1.2's drift detection, made visible.
//
// A missing capture is not a problem to report loudly: `capture` is not the
// command §10.2 installs, so on an ordinary install this file only appears if
// the user went looking for it.
func diagnosePayload(cfg *config.Config, caps style.Capabilities) payloadReport {
	var r payloadReport
	path, err := DefaultCapturePath()
	if err != nil {
		r.Problem = "no cache directory: " + err.Error()
		return r
	}
	r.Path = path

	info, err := os.Stat(path)
	if err != nil {
		r.Problem = "no capture yet; run `cc-statusline capture` in place of `render` to record one"
		return r
	}
	r.Age = time.Since(info.ModTime()).Truncate(time.Second).String()

	raw, err := os.ReadFile(path)
	if err != nil {
		r.Problem = err.Error()
		return r
	}
	// capture writes a wrapper — payload plus the environment it was rendered
	// in — so the payload has to be unwrapped before it can be decoded.
	var wrapper struct {
		Payload json.RawMessage `json:"payload"`
	}
	body := raw
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Payload) > 0 {
		body = wrapper.Payload
	}

	p, err := payload.Parse(body)
	if err != nil {
		r.Problem = "the captured payload did not decode: " + err.Error()
		return r
	}
	r.Decoded = true
	r.Unknown, r.Missing = p.KeyDiff()
	sort.Strings(r.Unknown)
	sort.Strings(r.Missing)

	ctx := line.Context{Payload: p, Config: cfg, Style: style.NewStyle(caps, cfg)}
	r.Rendered = line.RenderPlain(ctx)
	return r
}

func (r *report) writeText(w io.Writer, env map[string]string) {
	fmt.Fprintf(w, "cc-statusline %s\n", r.Version)

	fmt.Fprintln(w, "\ninstall")
	fmt.Fprintf(w, "  settings      %s%s\n", orNone(r.Install.SettingsPath), existsSuffix(r.Install.Exists))
	if r.Install.StatusLine != "" {
		fmt.Fprintf(w, "  statusLine    %s\n", r.Install.StatusLine)
		fmt.Fprintf(w, "  is this build %v\n", r.Install.IsUs)
	}
	if r.Install.Problem != "" {
		fmt.Fprintf(w, "  ! %s\n", r.Install.Problem)
	}

	fmt.Fprintln(w, "\nconfig")
	fmt.Fprintf(w, "  file          %s%s\n", orNone(r.Config.Path), existsSuffix(r.Config.Exists))
	if len(r.Config.Notes) == 0 {
		fmt.Fprintln(w, "  status        clean")
	}
	for _, n := range r.Config.Notes {
		fmt.Fprintf(w, "  ! %s\n", n)
	}

	c := r.Capabilities
	fmt.Fprintln(w, "\ncapabilities")
	fmt.Fprintf(w, "  icons         %s\n", c.Icons)
	fmt.Fprintf(w, "  powerline     %v\n", c.Powerline)
	fmt.Fprintf(w, "  color         %s\n", c.Profile)
	fmt.Fprintf(w, "  ambiguous     %d cell(s)\n", c.Ambiguous)
	fmt.Fprintf(w, "  columns       %d  (available after padding and reserve: %d)\n", c.Columns, c.Available)
	if len(c.Because) > 0 {
		keys := make([]string, 0, len(c.Because))
		for k := range c.Because {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		fmt.Fprintln(w, "  set here      "+strings.Join(keys, " "))
		for _, k := range keys {
			fmt.Fprintf(w, "    %-24s %s\n", k, c.Because[k])
		}
	} else {
		fmt.Fprintln(w, "  set here      nothing; every value above is a default")
	}

	fmt.Fprintln(w, "\npayload")
	if r.Payload.Problem != "" {
		fmt.Fprintf(w, "  ! %s\n", r.Payload.Problem)
	} else {
		fmt.Fprintf(w, "  capture       %s (%s old)\n", r.Payload.Path, r.Payload.Age)
	}
	// Unknown keys are never truncated: one of them is how a contract change
	// first shows up in the field (§3.1.2), and there are rarely more than a
	// handful. Missing keys are the opposite — a payload from an early session
	// is legitimately missing half the schema, and printing thirty-five names
	// buries the one line above that matters. The full lists are in --json.
	if len(r.Payload.Unknown) > 0 {
		fmt.Fprintf(w, "  ! new keys Claude Code sends that this build ignores: %s\n", strings.Join(r.Payload.Unknown, " "))
	}
	if len(r.Payload.Missing) > 0 {
		fmt.Fprintf(w, "  . keys this build knows about that the payload omitted: %s\n", elide(r.Payload.Missing, 6))
	}
	for _, l := range r.Payload.Rendered {
		fmt.Fprintf(w, "  > %s\n", l)
	}

	if r.LastError != "" {
		fmt.Fprintln(w, "\nlast error")
		for _, l := range strings.Split(r.LastError, "\n") {
			fmt.Fprintf(w, "  %s\n", l)
		}
	}
}

func orNone(s string) string {
	if s == "" {
		return "(cannot be located)"
	}
	return s
}

func elide(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, " ")
	}
	return fmt.Sprintf("%s … and %d more", strings.Join(items[:n], " "), len(items)-n)
}

func existsSuffix(exists bool) string {
	if exists {
		return ""
	}
	return "  (absent)"
}
