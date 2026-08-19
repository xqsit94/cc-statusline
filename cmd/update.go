package cmd

import (
	"flag"
	"fmt"
	"io"
	"runtime"

	"github.com/xqsit94/cc-statusline/internal/updater"
)

const updateRepo = "xqsit94/cc-statusline"

var (
	updaterLatest  = updater.Latest
	updaterFetch   = updater.Fetch
	updaterInstall = updater.Install
)

func Update(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(stderr)
	checkOnly := fs.Bool("check", false, "report the latest version without installing it")
	force := fs.Bool("force", false, "install the latest version without asking first")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	rel, err := updaterLatest(updateRepo)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "current    %s\n", version)
	fmt.Fprintf(stdout, "latest     %s\n", rel.TagName)

	if version != "dev" && updater.Compare(version, rel.TagName) >= 0 {
		fmt.Fprintln(stdout, "already running the latest version")
		return 0
	}

	if *checkOnly {
		fmt.Fprintf(stdout, "\nRun `cc-statusline update --force` to install %s.\n", rel.TagName)
		return 0
	}
	if !*force {
		fmt.Fprintf(stdout, "\n%s is available. Run `cc-statusline update --force` to install it.\n", rel.TagName)
		return 0
	}

	exe, err := selfPath()
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	binary, err := updaterFetch(updateRepo, rel.TagName, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}
	if err := updaterInstall(exe, binary); err != nil {
		fmt.Fprintf(stderr, "cc-statusline: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "installed  %s -> %s\n", exe, rel.TagName)
	return 0
}
