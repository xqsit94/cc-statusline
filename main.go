package main

import (
	"fmt"
	"os"

	"github.com/xqsit94/cc-statusline/cmd"
	"github.com/xqsit94/cc-statusline/internal/style"
)

func main() { os.Exit(run(os.Args[1:], style.Environ(os.Environ()))) }

func run(args []string, env map[string]string) (code int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "cc-statusline: internal error: %v\n", r)
			code = 1
		}
	}()
	return cmd.Main(args, env)
}
