package commands

import (
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
	"github.com/nonibytes/ministore/pkg/ministore/schema"
)

func RunIndex(g cliopt.GlobalOptions, argv []string) int {
	if len(argv) == 0 {
		fmt.Fprintln(os.Stderr, "index requires a subcommand: create|list|schema|migrate|stats|optimize|drop")
		return 2
	}
	verb := argv[0]
	args := argv[1:]
	switch verb {
	case "create":
		return runIndexCreate(g, args)
	case "list":
		return runIndexList(g, args)
	case "schema":
		return runIndexSchema(g, args)
	case "migrate":
		return runIndexMigrate(g, args)
	case "stats":
		return runIndexStats(g, args)
	case "optimize":
		return runIndexOptimize(g, args)
	case "drop":
		return runIndexDrop(g, args)
	case "--help", "-h", "help":
		fmt.Fprintln(os.Stdout, "index subcommands: create|list|schema|migrate|stats|optimize|drop")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown index subcommand: %s\n", verb)
		return 2
	}
}

func runIndexCreate(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("index create", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName string
	var schemaPath string
	fs.StringVar(&indexName, "index", "", "index name")
	fs.StringVar(&indexName, "i", "", "index name")
	fs.StringVar(&schemaPath, "schema", "", "schema json file")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if indexName == "" {
		fmt.Fprintln(os.Stderr, "missing --index")
		return 2
	}
	if schemaPath == "" {
		fmt.Fprintln(os.Stderr, "missing --schema (skeleton: inline --field not implemented yet)")
		return 2
	}
	b, err := os.ReadFile(schemaPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sch, err := schema.FromJSON(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := sch.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx := ministore.Background()
	client, err := ministore.Open(ctx, ministore.OpenOptionsFromCLI(g))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_, err = client.CreateIndex(ctx, cliutil.ResolveIndexRef(g, indexName), sch, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "created")
	return 0
}

func runIndexList(g cliopt.GlobalOptions, argv []string) int {
	ctx := ministore.Background()
	client, err := ministore.Open(ctx, ministore.OpenOptionsFromCLI(g))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	infos, err := client.ListIndexes(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cliutil.PrintJSON(os.Stdout, infos)
	return 0
}

func runIndexSchema(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("index schema", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName string
	var applyPath string
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&applyPath, "apply", "", "apply schema json (additive)")
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
	if applyPath == "" {
		cliutil.PrintJSON(os.Stdout, idx.Schema())
		return 0
	}
	b, err := os.ReadFile(applyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ns, err := schema.FromJSON(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := idx.ApplySchemaAdditive(ctx, ns); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "schema applied")
	return 0
}

func runIndexMigrate(g cliopt.GlobalOptions, argv []string) int {
	fmt.Fprintln(os.Stderr, "migrate: TODO (skeleton)")
	return 2
}

func runIndexStats(g cliopt.GlobalOptions, argv []string) int {
	fmt.Fprintln(os.Stderr, "index stats: TODO (skeleton)")
	return 2
}

func runIndexOptimize(g cliopt.GlobalOptions, argv []string) int {
	fmt.Fprintln(os.Stderr, "index optimize: TODO (skeleton)")
	return 2
}

func runIndexDrop(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("index drop", flag.ContinueOnError)
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
	if err := client.DropIndex(ctx, cliutil.ResolveIndexRef(g, indexName)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintln(os.Stdout, "dropped")
	return 0
}
