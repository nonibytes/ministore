package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
)

func RunDelete(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName, path, where string
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&path, "path", "", "path")
	fs.StringVar(&path, "p", "", "path")
	fs.StringVar(&where, "where", "", "where query")
	fs.StringVar(&where, "w", "", "where query")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if indexName == "" {
		fmt.Fprintln(os.Stderr, "missing --index")
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
	if path != "" {
		ok, err := idx.Delete(ctx, path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if ok {
			fmt.Fprintln(os.Stdout, "deleted")
		} else {
			fmt.Fprintln(os.Stdout, "not found")
		}
		return 0
	}
	if where != "" {
		n, err := idx.DeleteWhere(ctx, where)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintf(os.Stdout, "deleted %d\n", n)
		return 0
	}
	fmt.Fprintln(os.Stderr, "provide --path or --where")
	return 2
}
