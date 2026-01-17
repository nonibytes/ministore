package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
)

func RunGet(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName, path string
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&path, "path", "", "path")
	fs.StringVar(&path, "p", "", "path")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if indexName == "" || path == "" {
		fmt.Fprintln(os.Stderr, "missing --index or --path")
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
	view, err := idx.Get(ctx, path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cliutil.PrintJSON(os.Stdout, view.Doc)
	return 0
}
