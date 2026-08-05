package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

// Render implements `cc-statusline render`. It is the hot path, and it holds
// PRD §3.3's failure contract:
//
//   - Exit 0 for every input. A non-zero exit blanks the status line, and the
//     user cannot tell that from "nothing to report".
//   - At least one non-empty line on stdout, always.
//   - Nothing on stderr.
//   - One Write. Both lines are assembled in a buffer first, so a failure
//     halfway through cannot emit a torn line.
//   - No goroutines. A panic in a goroutine that does not recover in that same
//     goroutine kills the process regardless of what this function defers, so
//     the render path spawns none — ever.
//
// The single exception is `--payload FILE`, which is a human debugging aid
// rather than the path Claude Code invokes: an unreadable file exits 1 and
// explains itself on stderr, because silently rendering someone else's stdin
// would be worse than a visible failure.
func Render(args []string, env map[string]string, stdin io.Reader, stdout io.Writer) (code int) {
	var out bytes.Buffer
	var p *payload.Payload

	// The fallback's marker is a variable the deferred recover closes over, and
	// it starts at the Unicode glyph so that the recover can never need
	// anything that has not already been computed. It is narrowed to the
	// resolved icon set below, once config exists — but a panic before that
	// point still has a marker to print.
	marker := glyphFallback

	defer func() {
		if r := recover(); r != nil {
			// Reset first: the buffer may hold a half-written line, and the
			// fallback has to be the only thing on stdout.
			out.Reset()
			out.WriteString(fallback(p, marker))
			out.WriteByte('\n')
			code = 0
		}
		stdout.Write(out.Bytes())
	}()

	src, exit, ok := payloadSource(args, stdin)
	if !ok {
		return exit
	}

	// A decode error is not a render failure. Parse always returns a usable
	// payload, so an unparseable stdin renders the fallback rather than
	// nothing. The error itself becomes visible through `doctor` at M5.
	p, _ = payload.Decode(src)

	// Loaded once, here, and handed down. Loading inside renderLines instead
	// would read the config file twice on the hot path — once for the line and
	// once for the fallback's glyph — for no gain.
	cfg, notes := config.Load(env)

	// §7.1: the notes have nowhere to go on a status line, so they go to a file
	// and `doctor` reads them back. This is the only thing on the render path
	// that touches the filesystem for a reason other than the payload, and it
	// does nothing at all in the overwhelmingly common case of a config with no
	// problems in it.
	recordConfigNotes(notes)

	caps := style.Detect(env, cfg)
	marker = style.GlyphsFor(caps.Icons, cfg).ModelMarker

	lines := renderLines(p, cfg, caps, env)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}

	// Last guard before the write: every line was empty, or there were none.
	// Every segment can report absent at once — an empty payload does exactly
	// that — and the contract still says one non-empty line.
	if out.Len() == 0 {
		out.WriteString(fallback(p, marker))
		out.WriteByte('\n')
	}
	return 0
}

// glyphFallback is the marker used before the icon set is known. It is the
// Unicode column of PRD §6.2, which is also the default.
const glyphFallback = "◆"

// renderLines builds the status line body.
//
// It is a variable rather than a function so that tests can install a panicking
// implementation and prove the recover above actually holds — a contract
// nothing else can exercise, since correct code never panics.
var renderLines func(*payload.Payload, *config.Config, style.Capabilities, map[string]string) []string = defaultRenderLines

// defaultRenderLines is PRD §4.4's pipeline. Every arrow in that diagram is
// in-process: nothing forks, nothing dials, nothing awaits.
//
// Its config comes from Render, already loaded and validated. config.Load
// cannot fail: an unreadable file, a syntax error, an unknown key, an
// out-of-range value — each becomes a note against a config that is still
// complete and renderable (PRD §7.1). Render drops the notes because it has
// nowhere to display them; M5 routes them to last-error.txt and `doctor`
// reports them.
func defaultRenderLines(p *payload.Payload, cfg *config.Config, caps style.Capabilities, env map[string]string) []string {
	ctx := line.Context{
		Payload: p,
		Config:  cfg,
		Style:   style.NewStyle(caps, cfg),
		Git:     discoverGit(env, p, cfg),
	}
	return line.Render(ctx)
}

// discoverGit resolves the branch once, before the segment loop.
//
// The starting directory is the payload's workspace.current_dir, never
// os.Getwd(): a session whose directory was deleted underneath it makes Getwd
// return ENOENT, and the branch would disappear for a reason unrelated to git.
//
// CC_STATUSLINE_NO_GIT is not read here. config.Load has already folded it into
// git.enabled, which is the point of §7.3's overlay: one switch, checked once,
// so the wizard's preview and the real render cannot disagree about it.
func discoverGit(env map[string]string, p *payload.Payload, cfg *config.Config) gitinfo.Info {
	if !cfg.Git.Enabled {
		return gitinfo.Info{}
	}
	dir, ok := p.CurrentDir()
	if !ok {
		return gitinfo.Info{}
	}
	return gitinfo.Discover(env, dir)
}

// fallback is PRD §3.3's last line of defence: the model name if anything at
// all survived decoding, otherwise the binary's own name so that the status
// line is occupied by something a user can search for.
//
// The marker is passed in rather than resolved here so that this function
// cannot fail. It runs from inside a recover, where a second panic would take
// the process down with nothing on stdout at all — which is the one outcome
// §3.3 exists to prevent.
//
// It is deliberately unstyled. Colour would mean a Style, and a Style is built
// from the config and capabilities that may be exactly what just panicked.
func fallback(p *payload.Payload, marker string) string {
	if p != nil {
		if name, ok := p.ModelName(); ok && strings.TrimSpace(name) != "" {
			return marker + " " + name
		}
	}
	return "cc-statusline"
}

// payloadSource resolves where the payload comes from. It returns ok=false
// only for the --payload exception documented on Render.
func payloadSource(args []string, stdin io.Reader) (src io.Reader, exit int, ok bool) {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	// Discard: PRD §3.3 forbids stderr here, and flag's default output is it.
	fs.SetOutput(io.Discard)
	file := fs.String("payload", "", "read the payload from FILE instead of stdin")

	if err := fs.Parse(args); err != nil {
		// A malformed flag is a human's typo in settings.json. Rendering from
		// stdin keeps the contract; M5's `doctor` is where it becomes visible.
		return stdin, 0, true
	}
	if *file == "" {
		return stdin, 0, true
	}

	f, err := os.Open(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cc-statusline: --payload: %v\n", err)
		return nil, 1, false
	}
	// Not closed: render is a one-shot process and the file descriptor dies
	// with it. Closing here would need the reader to outlive this function.
	return f, 0, true
}
