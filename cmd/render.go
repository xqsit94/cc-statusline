package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/xqsit94/cc-statusline/internal/config"
	"github.com/xqsit94/cc-statusline/internal/gitinfo"
	"github.com/xqsit94/cc-statusline/internal/line"
	"github.com/xqsit94/cc-statusline/internal/payload"
	"github.com/xqsit94/cc-statusline/internal/style"
)

func Render(args []string, env map[string]string, stdin io.Reader, stdout io.Writer) (code int) {
	var out bytes.Buffer
	var p *payload.Payload

	marker := glyphFallback

	defer func() {
		if r := recover(); r != nil {
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

	p, _ = payload.Decode(src)

	cfg, notes := config.Load(env)

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

	if out.Len() == 0 {
		out.WriteString(fallback(p, marker))
		out.WriteByte('\n')
	}
	return 0
}

const glyphFallback = "◆"

var renderLines func(*payload.Payload, *config.Config, style.Capabilities, map[string]string) []string = defaultRenderLines

func defaultRenderLines(p *payload.Payload, cfg *config.Config, caps style.Capabilities, env map[string]string) []string {
	ctx := line.Context{
		Payload: p,
		Config:  cfg,
		Style:   style.NewStyle(caps, cfg),
		Git:     discoverGit(env, p, cfg),
		Zone:    time.Local,
	}
	return line.Render(ctx)
}

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

func fallback(p *payload.Payload, marker string) string {
	if p != nil {
		if name, ok := p.ModelName(); ok && strings.TrimSpace(name) != "" {
			return marker + " " + name
		}
	}
	return "cc-statusline"
}

func payloadSource(args []string, stdin io.Reader) (src io.Reader, exit int, ok bool) {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	file := fs.String("payload", "", "read the payload from FILE instead of stdin")

	if err := fs.Parse(args); err != nil {
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
	return f, 0, true
}
