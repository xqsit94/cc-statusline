package cmd

import (
	"fmt"
	"io"
	"runtime/debug"
)

// Build metadata, injected by GoReleaser at M5 via -ldflags. The defaults are
// what a `go build` or `go install` produces, and vcsFallback below recovers
// most of it from the embedded build info rather than reporting "none".
var (
	version = "dev"
	commit  = ""
	date    = ""
)

// Version implements `cc-statusline version`.
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

// vcsFallback reads the VCS stamps the Go toolchain embeds automatically. A
// binary built with plain `go build` still knows its commit; reporting "none"
// when the answer is right there makes bug reports harder than they need to be.
func vcsFallback() (commit, date string) {
	commit, date = "unknown", "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return commit, date
	}

	// Collected before composing: debug.BuildSetting order is not specified,
	// so appending "-dirty" inside the loop can land on "unknown".
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
