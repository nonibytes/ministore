package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cli/commands"
	"github.com/nonibytes/ministore/internal/cliopt"
)

// Execute runs the CLI and returns an exit code.
func Execute(argv []string) int {
	globalFS := flag.NewFlagSet("ministore", flag.ContinueOnError)
	globalFS.SetOutput(os.Stderr)
	g := cliopt.DefaultGlobalOptions()
	cliopt.BindGlobalFlags(globalFS, &g)

	if err := globalFS.Parse(argv); err != nil {
		// flag package already printed the error
		return 2
	}

	args := globalFS.Args()
	if len(args) == 0 {
		PrintRootHelp(os.Stdout)
		return 0
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "--help", "-h", "help":
		PrintRootHelp(os.Stdout)
		return 0
	case "index":
		return commands.RunIndex(g, rest)
	case "put":
		return commands.RunPut(g, rest)
	case "get":
		return commands.RunGet(g, rest)
	case "peek":
		return commands.RunPeek(g, rest)
	case "delete":
		return commands.RunDelete(g, rest)
	case "search":
		return commands.RunSearch(g, rest)
	case "discover":
		return commands.RunDiscover(g, rest)
	case "stats":
		return commands.RunStats(g, rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", verb)
		PrintRootHelp(os.Stderr)
		return 2
	}
}
