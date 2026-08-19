package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/xqsit94/cc-statusline/internal/spike"
)

func Spike(args []string, stdin io.Reader, stdout io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "cc-statusline: spike needs a mode: capture or report\n")
		return 2
	}

	switch args[0] {
	case "capture":
		return spike.Capture(args[1:])
	case "report":
		return spike.Report(stdout)
	default:
		fmt.Fprintf(os.Stderr, "cc-statusline: unknown spike mode %q\n", args[0])
		return 2
	}
}
