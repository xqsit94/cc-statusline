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

// Uninstall implements `cc-statusline uninstall` — PRD §10.3.
//
// It is `init` run backwards through the same machinery: the same plain-JSON
// gate, the same backup, the same atomic write. It removes one key and leaves
// every other byte of settings.json alone.
//
// **It does not restore a backup**, and §10.3 explains why: backups are stamped
// per `init`, so restoring the newest would revert every settings.json edit the
// user made since installing — permissions, hooks, environment — not just the
// status line. The backups stay on disk as a manual escape hatch, and the
// removal itself makes a fresh one.
//
// The config file is left in place. Uninstalling the status line is not a
// statement about wanting to lose a file you spent an afternoon tuning.
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

	// Same order as init, for the same reason: on a file with comments, the
	// value read below could come from inside one.
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

	// Refusing to remove somebody else's status line is not in §10.3, and it is
	// the difference between "uninstall me" and "uninstall whatever is there".
	// A user who tried this tool, went back to another one, and later tidied up
	// with `cc-statusline uninstall` should not silently lose that other one's
	// configuration.
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

// mentionsThisBinary reports whether an existing statusLine command looks like
// ours.
//
// It matches on the basename rather than the full path deliberately: a user who
// moved the binary, reinstalled it under a different prefix, or upgraded from
// ~/.local/bin to /usr/local/bin should still be able to uninstall. The cost of
// being loose here is that a program of the same name elsewhere would match,
// which is a much smaller problem than an uninstall that cannot uninstall.
func mentionsThisBinary(rawStatusLine string) bool {
	program := firstArg(commandOf(rawStatusLine))
	if program == "" {
		return false
	}
	return strings.HasPrefix(filepath.Base(program), "cc-statusline")
}

// firstArg extracts the program from a shell command line.
//
// strings.Fields is not enough, because `init` shell-quotes the path: on macOS
// the binary can live under "Application Support", and the value we wrote is
// `'/Users/x/Library/Application Support/bin/cc-statusline' render`. Splitting
// that on whitespace yields `'/Users/x/Library/Application` and an uninstall
// that cannot recognise its own installation.
//
// It understands the three things shellQuote can emit — a single-quoted string,
// the backslash escape shellQuote uses for an embedded apostrophe, and a bare
// path — and treats double quotes the same way, for the benefit of a command
// somebody wrote by hand.
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
