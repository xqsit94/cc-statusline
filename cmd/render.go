package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/xqsit94/cc-statusline/internal/payload"
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
func Render(args []string, stdin io.Reader, stdout io.Writer) (code int) {
	var out bytes.Buffer
	var p *payload.Payload

	defer func() {
		if r := recover(); r != nil {
			// Reset first: the buffer may hold a half-written line, and the
			// fallback has to be the only thing on stdout.
			out.Reset()
			out.WriteString(fallback(p))
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

	lines := renderLines(p)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}

	// Last guard before the write: every line was empty, or there were none.
	// M2's segments can all report absent at once — an empty payload does
	// exactly that — and the contract still says one non-empty line.
	if out.Len() == 0 {
		out.WriteString(fallback(p))
		out.WriteByte('\n')
	}
	return 0
}

// renderLines builds the status line body.
//
// M1 emits the model line only; M2 replaces this with the segment pipeline of
// PRD §4.4. It is a variable rather than a function so that tests can install a
// panicking implementation and prove the recover above actually holds — a
// contract nothing else can exercise, since correct code never panics.
var renderLines = defaultRenderLines

func defaultRenderLines(p *payload.Payload) []string {
	return []string{fallback(p)}
}

// fallback is PRD §3.3's last line of defence: the model name if anything at
// all survived decoding, otherwise the binary's own name so that the status
// line is occupied by something a user can search for.
//
// The marker is hardcoded here. M3 routes it through the glyph set so ASCII
// mode degrades it; until then a fallback under CLAUDE_STATUSLINE_ASCII=1 can
// still emit `◆`, which is a cosmetic bug in a path that should almost never
// run.
func fallback(p *payload.Payload) string {
	if p != nil {
		if name, ok := p.ModelName(); ok && strings.TrimSpace(name) != "" {
			return "◆ " + name
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
