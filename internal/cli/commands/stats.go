package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
)

func RunStats(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("stats", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName, field, where string
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&field, "field", "", "field")
	fs.StringVar(&where, "where", "", "where")
	fs.StringVar(&where, "w", "", "where")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if indexName == "" || field == "" {
		fmt.Fprintln(os.Stderr, "missing --index or --field")
		return 2
	}
	ctx := ministore.Background()
	client, err := ministore.Open(ctx, ministore.OpenOptionsFromCLI(g))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	idx, err := client.OpenIndex(ctx, cliutil.ResolveIndexRef(g, indexName), ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	out, err := idx.Stats(ctx, field, where)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cliutil.PrintJSON(os.Stdout, out)
	return 0
}
