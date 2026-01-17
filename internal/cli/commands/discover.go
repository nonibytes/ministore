package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
)

func RunDiscover(g cliopt.GlobalOptions, argv []string) int {
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "discover requires a subcommand: fields|values")
		return 2
	}
	sub := argv[0]
	args := argv[1:]
	switch sub {
	case "fields":
		return runDiscoverFields(g, args)
	case "values":
		return runDiscoverValues(g, args)
	default:
		fmt.Fprintln(os.Stderr, "unknown discover subcommand")
		return 2
	}
}

func runDiscoverFields(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("discover fields", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName string
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
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
	rows, err := idx.DiscoverFields(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cliutil.PrintJSON(os.Stdout, rows)
	return 0
}

func runDiscoverValues(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("discover values", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName, field, where string
	var top int
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&field, "field", "", "field")
	fs.StringVar(&where, "where", "", "where")
	fs.StringVar(&where, "w", "", "where")
	fs.IntVar(&top, "top", 100, "top")
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
	vals, err := idx.DiscoverValues(ctx, field, where, top)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cliutil.PrintJSON(os.Stdout, vals)
	return 0
}
