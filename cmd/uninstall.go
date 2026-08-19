package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/xqsit94/cc-statusline/internal/settings"
)

func Uninstall(args []string, env map[string]string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(stderr)
	force := fs.Bool("force", false, "remove the statusLine key even if it runs some other program")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	file, err := settings.Read(settings.Path(env))
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	if !file.Editable() {
		fmt.Fprintf(stdout, `%s is not plain JSON — it contains comments or a trailing comma.

Remove the "statusLine" key yourself; editing it automatically is not safe
here. See `+"`cc-statusline init`"+` for the reasoning.
`, file.Target)
		return 0
	}

	existing, had := file.StatusLine()
	if !had {
		fmt.Fprintf(stdout, "settings   %s (no statusLine key; nothing to do)\n", file.Target)
		return 0
	}

	if !*force && !mentionsThisBinary(existing) {
		fmt.Fprintf(stdout, `settings   %s (left alone)

The statusLine key runs something else:
  %s

Remove it with `+"`cc-statusline uninstall --force`"+` if that is what you want.
`, file.Target, strings.TrimSpace(commandOf(existing)))
		return 0
	}

	stripped, err := file.Unpatch()
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}
	backup, err := settings.Backup(file, time.Now().UTC().Format("20060102T150405Z"))
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: cannot back up %s: %v\n", file.Target, err)
		return 1
	}
	if err := settings.Write(file, stripped); err != nil {
		fmt.Fprintf(stderr, "cc-statusline: cannot write %s: %v\n", file.Target, err)
		return 1
	}

	fmt.Fprintf(stdout, "settings   %s (statusLine removed)\n", file.Target)
	if backup != "" {
		fmt.Fprintf(stdout, "backup     %s\n", backup)
	}
	fmt.Fprintln(stdout, "config     left in place; delete it yourself if you want it gone")
	return 0
}

func mentionsThisBinary(rawStatusLine string) bool {
	program := firstArg(commandOf(rawStatusLine))
	if program == "" {
		return false
	}
	return strings.HasPrefix(filepath.Base(program), "cc-statusline")
}

func firstArg(cmd string) string {
	var b strings.Builder
	var quote byte
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				b.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '\\' && i+1 < len(cmd):
			i++
			b.WriteByte(cmd[i])
		case c == ' ' || c == '\t':
			if b.Len() > 0 {
				return b.String()
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func commandOf(rawStatusLine string) string {
	var d settings.Desired
	if err := json.Unmarshal([]byte(rawStatusLine), &d); err != nil {
		return ""
	}
	return d.Command
}
