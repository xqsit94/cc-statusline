package cmd

import (
	"fmt"
	"io"
	"runtime/debug"
)

var (
	version = "dev"
	commit  = ""
	date    = ""
)

func Version(w io.Writer) int {
	c, d := commit, date
	if c == "" || d == "" {
		vcsCommit, vcsDate := vcsFallback()
		if c == "" {
			c = vcsCommit
		}
		if d == "" {
			d = vcsDate
		}
	}
	fmt.Fprintf(w, "cc-statusline %s (%s, %s)\n", version, c, d)
	return 0
}

func vcsFallback() (commit, date string) {
	commit, date = "unknown", "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return commit, date
	}

	var revision string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			if s.Value != "" {
				date = s.Value
			}
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	if revision != "" {
		commit = revision
		if len(commit) > 7 {
			commit = commit[:7]
		}
		if dirty {
			commit += "-dirty"
		}
	}
	return commit, date
}
