package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/postgres"
	"github.com/ministore/ministore/ministore/storage/sqlite"
	_ "modernc.org/sqlite"
)

// setArgs is a custom flag type for repeatable --set flags
type setArgs []string

func (s *setArgs) String() string { return strings.Join(*s, ",") }
func (s *setArgs) Set(v string) error {
	*s = append(*s, v)
	return nil
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	ctx := context.Background()

	switch command {
	case "index":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ministore index <create|schema|optimize>")
			os.Exit(1)
		}
		handleIndex(ctx, os.Args[2:])
	case "put":
		handlePut(ctx, os.Args[2:])
	case "get":
		handleGet(ctx, os.Args[2:])
	case "peek":
		handlePeek(ctx, os.Args[2:])
	case "delete":
		handleDelete(ctx, os.Args[2:])
	case "search":
		handleSearch(ctx, os.Args[2:])
	case "discover":
		if len(os.Args) < 3 {
			fmt.Println("Usage: ministore discover <fields|values>")
			os.Exit(1)
		}
		handleDiscover(ctx, os.Args[2:])
	case "stats":
		handleStats(ctx, os.Args[2:])
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("ministore - A general-purpose search index")
	fmt.Println("\nUsage:")
	fmt.Println("  ministore index create -i <path> --schema <schema.json> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore index schema -i <path> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore index optimize -i <path> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore put -i <path> --json [--backend sqlite|postgres] [--schema-name <name>]     (read JSON lines from stdin)")
	fmt.Println("  ministore put -i <path> -p <item-path> --set key=value... [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore get -i <path> -p <item-path> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore peek -i <path> -p <item-path> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore delete -i <path> -p <item-path> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore delete -i <path> -w <query> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore search -i <path> -w <query> [--limit N] [--show all|field1,field2] [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore discover fields -i <path> [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore discover values -i <path> --field <field> [--top N] [-w <query>] [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("  ministore stats -i <path> --field <field> [-w <query>] [--backend sqlite|postgres] [--schema-name <name>]")
	fmt.Println("\nBackends:")
	fmt.Println("  sqlite   - SQLite file database (default)")
	fmt.Println("  postgres - PostgreSQL database (requires connection string)")
	fmt.Println("\nFor PostgreSQL, -i is the connection string: postgresql://user:pass@host:port/dbname")
	fmt.Println("Use --schema-name to specify the PostgreSQL schema (defaults to 'ministore')")
}

// createAdapter creates the appropriate storage adapter based on backend flag
func createAdapter(backend, indexPath, schemaName string) storage.Adapter {
	switch backend {
	case "postgres", "pg":
		if schemaName == "" {
			schemaName = "ministore"
		}
		return postgres.New(indexPath, schemaName)
	default:
		return sqlite.New(indexPath)
	}
}

func handleIndex(ctx context.Context, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ministore index <create|schema|optimize>")
		os.Exit(1)
	}

	subcmd := args[0]

	switch subcmd {
	case "create":
		fs := flag.NewFlagSet("index create", flag.ExitOnError)
		indexPath := fs.String("i", "", "index path (required)")
		schemaFile := fs.String("schema", "", "schema JSON file (required)")
		backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
		schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
		fs.Parse(args[1:])

		if *indexPath == "" || *schemaFile == "" {
			fs.Usage()
			os.Exit(1)
		}

		// Read schema file
		schemaData, err := os.ReadFile(*schemaFile)
		if err != nil {
			fmt.Printf("Error reading schema file: %v\n", err)
			os.Exit(1)
		}

		var schema ministore.Schema
		if err := json.Unmarshal(schemaData, &schema); err != nil {
			fmt.Printf("Error parsing schema: %v\n", err)
			os.Exit(1)
		}

		adapter := createAdapter(*backend, *indexPath, *schemaName)
		ix, err := ministore.Create(ctx, adapter, schema, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Printf("Error creating index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		fmt.Printf("Created index at: %s\n", *indexPath)
		if *backend == "postgres" || *backend == "pg" {
			fmt.Printf("Schema: %s\n", *schemaName)
		}
		fmt.Printf("Fields: %d\n", len(schema.Fields))

	case "schema":
		fs := flag.NewFlagSet("index schema", flag.ExitOnError)
		indexPath := fs.String("i", "", "index path (required)")
		backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
		schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
		fs.Parse(args[1:])

		if *indexPath == "" {
			fs.Usage()
			os.Exit(1)
		}

		adapter := createAdapter(*backend, *indexPath, *schemaName)
		ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Printf("Error opening index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		schema := ix.Schema()
		schemaJSON, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			fmt.Printf("Error encoding schema: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(string(schemaJSON))

	case "optimize":
		fs := flag.NewFlagSet("index optimize", flag.ExitOnError)
		indexPath := fs.String("i", "", "index path (required)")
		backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
		schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
		fs.Parse(args[1:])

		if *indexPath == "" {
			fs.Usage()
			os.Exit(1)
		}

		adapter := createAdapter(*backend, *indexPath, *schemaName)
		ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Printf("Error opening index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		if err := ix.Optimize(ctx); err != nil {
			fmt.Printf("Error optimizing index: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Index optimized")

	default:
		fmt.Printf("Unknown index command: %s\n", subcmd)
		os.Exit(1)
	}
}

func handlePut(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("put", flag.ExitOnError)
	indexPath := fs.String("i", "", "index path (required)")
	itemPath := fs.String("p", "", "item path (for single put with --set)")
	jsonMode := fs.Bool("json", false, "read JSON lines from stdin")
	backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
	schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")

	var sets setArgs
	fs.Var(&sets, "set", "set field value key=value (repeatable)")

	fs.Parse(args)

	if *indexPath == "" {
		fs.Usage()
		os.Exit(1)
	}

	adapter := createAdapter(*backend, *indexPath, *schemaName)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Printf("Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	if *jsonMode {
		// Read JSON lines from stdin
		scanner := bufio.NewScanner(os.Stdin)
		count := 0
		for scanner.Scan() {
			line := scanner.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			if err := ix.PutJSON(ctx, []byte(line)); err != nil {
				fmt.Printf("Error putting item: %v\n", err)
				os.Exit(1)
			}
			count++
		}
		if err := scanner.Err(); err != nil {
			fmt.Printf("Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Put %d items\n", count)
	} else if *itemPath != "" {
		// Single put with --set flags
		doc := map[string]any{"path": *itemPath}
		for _, kv := range sets {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				fmt.Printf("Invalid --set %q (expected key=value)\n", kv)
				os.Exit(1)
			}
			doc[parts[0]] = parts[1]
		}

		docJSON, err := json.Marshal(doc)
		if err != nil {
			fmt.Printf("Error encoding document: %v\n", err)
			os.Exit(1)
		}

		if err := ix.PutJSON(ctx, docJSON); err != nil {
			fmt.Printf("Error putting item: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Put item: %s\n", *itemPath)
	} else {
		fmt.Println("Either --json or -p with --set flags required")
		os.Exit(1)
	}
}

func handleGet(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("get", flag.ExitOnError)
	indexPath := fs.String("i", "", "index path (required)")
	itemPath := fs.String("p", "", "item path (required)")
	backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
	schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
	fs.Parse(args)

	if *indexPath == "" || *itemPath == "" {
		fs.Usage()
		os.Exit(1)
	}

	adapter := createAdapter(*backend, *indexPath, *schemaName)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Printf("Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	item, err := ix.Get(ctx, *itemPath)
	if err != nil {
		if ministore.IsKind(err, ministore.ErrNotFound) {
			fmt.Printf("Item not found: %s\n", *itemPath)
			os.Exit(1)
		}
		fmt.Printf("Error getting item: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Path: %s\n", item.Path)
	fmt.Printf("Created: %d\n", item.Meta.CreatedAtMS)
	fmt.Printf("Updated: %d\n", item.Meta.UpdatedAtMS)
	fmt.Printf("\nData:\n%s\n", string(item.DocJSON))
}

func handlePeek(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("peek", flag.ExitOnError)
	indexPath := fs.String("i", "", "index path (required)")
	itemPath := fs.String("p", "", "item path (required)")
	backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
	schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
	fs.Parse(args)

	if *indexPath == "" || *itemPath == "" {
		fs.Usage()
		os.Exit(1)
	}

	adapter := createAdapter(*backend, *indexPath, *schemaName)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Printf("Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	data, err := ix.Peek(ctx, *itemPath)
	if err != nil {
		if ministore.IsKind(err, ministore.ErrNotFound) {
			fmt.Printf("Item not found: %s\n", *itemPath)
			os.Exit(1)
		}
		fmt.Printf("Error peeking item: %v\n", err)
		os.Exit(1)
	}

	// Pretty print JSON
	var obj any
	if err := json.Unmarshal(data, &obj); err == nil {
		pretty, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Println(string(data))
	}
}

func handleDelete(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("delete", flag.ExitOnError)
	indexPath := fs.String("i", "", "index path (required)")
	itemPath := fs.String("p", "", "item path (for single delete)")
	where := fs.String("w", "", "query for batch delete")
	backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
	schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
	fs.Parse(args)

	if *indexPath == "" {
		fs.Usage()
		os.Exit(1)
	}

	if *itemPath == "" && *where == "" {
		fmt.Println("Either -p or -w required")
		os.Exit(1)
	}

	adapter := createAdapter(*backend, *indexPath, *schemaName)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Printf("Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	if *itemPath != "" {
		deleted, err := ix.Delete(ctx, *itemPath)
		if err != nil {
			fmt.Printf("Error deleting item: %v\n", err)
			os.Exit(1)
		}
		if deleted {
			fmt.Printf("Deleted: %s\n", *itemPath)
		} else {
			fmt.Printf("Item not found: %s\n", *itemPath)
		}
	} else {
		count, err := ix.DeleteWhere(ctx, *where)
		if err != nil {
			fmt.Printf("Error deleting items: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted %d items\n", count)
	}
}

func handleSearch(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("search", flag.ExitOnError)
	indexPath := fs.String("i", "", "index path (required)")
	where := fs.String("w", "", "query (required)")
	limit := fs.Int("limit", 20, "max results")
	show := fs.String("show", "none", "output fields: none, all, or comma-separated field names")
	rank := fs.String("rank", "default", "ranking: default, recency, none, or field:<name>")
	explain := fs.Bool("explain", false, "show query plan")
	after := fs.String("after", "", "cursor for pagination")
	format := fs.String("format", "pretty", "output format: pretty or json")
	backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
	schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
	fs.Parse(args)

	if *indexPath == "" || *where == "" {
		fs.Usage()
		os.Exit(1)
	}

	adapter := createAdapter(*backend, *indexPath, *schemaName)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Printf("Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	opts := ministore.SearchOptions{
		Limit:   *limit,
		After:   *after,
		Explain: *explain,
	}

	// Parse show option
	switch *show {
	case "none":
		opts.Show.Kind = ministore.ShowNone
	case "all":
		opts.Show.Kind = ministore.ShowAll
	default:
		opts.Show.Kind = ministore.ShowFields
		opts.Show.Fields = strings.Split(*show, ",")
	}

	// Parse rank option
	switch {
	case *rank == "default":
		opts.Rank.Kind = ministore.RankDefault
	case *rank == "recency":
		opts.Rank.Kind = ministore.RankRecency
	case *rank == "none":
		opts.Rank.Kind = ministore.RankNone
	case strings.HasPrefix(*rank, "field:"):
		opts.Rank.Kind = ministore.RankField
		opts.Rank.Field = strings.TrimPrefix(*rank, "field:")
	default:
		fmt.Printf("Unknown rank mode: %s\n", *rank)
		os.Exit(1)
	}

	result, err := ix.Search(ctx, *where, opts)
	if err != nil {
		fmt.Printf("Error searching: %v\n", err)
		os.Exit(1)
	}

	// JSON output mode
	if *format == "json" {
		output := map[string]any{
			"items":    make([]any, 0, len(result.Items)),
			"has_more": result.HasMore,
		}
		if result.NextCursor != "" {
			output["next_cursor"] = result.NextCursor
		}

		for _, item := range result.Items {
			var obj any
			if err := json.Unmarshal(item, &obj); err == nil {
				output["items"] = append(output["items"].([]any), obj)
			}
		}

		jsonOut, _ := json.Marshal(output)
		fmt.Println(string(jsonOut))
		return
	}

	// Pretty output mode
	if *explain {
		fmt.Println("=== Query Plan ===")
		for _, step := range result.ExplainSteps {
			fmt.Printf("  %s\n", step)
		}
		fmt.Println("\n=== SQL ===")
		fmt.Println(result.ExplainSQL)
		fmt.Println("\n=== Results ===")
	}

	// Print results
	for _, item := range result.Items {
		var obj any
		if err := json.Unmarshal(item, &obj); err == nil {
			pretty, _ := json.MarshalIndent(obj, "", "  ")
			fmt.Println(string(pretty))
		} else {
			fmt.Println(string(item))
		}
	}

	// Print pagination info
	fmt.Printf("\n--- %d results", len(result.Items))
	if result.HasMore {
		fmt.Printf(", more available")
		if result.NextCursor != "" {
			fmt.Printf(" (cursor: %s)", result.NextCursor)
		}
	}
	fmt.Println(" ---")
}

func handleDiscover(ctx context.Context, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: ministore discover <fields|values>")
		os.Exit(1)
	}

	subcmd := args[0]

	switch subcmd {
	case "fields":
		fs := flag.NewFlagSet("discover fields", flag.ExitOnError)
		indexPath := fs.String("i", "", "index path (required)")
		backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
		schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
		format := fs.String("format", "pretty", "output format: pretty or json")
		fs.Parse(args[1:])

		if *indexPath == "" {
			fs.Usage()
			os.Exit(1)
		}

		adapter := createAdapter(*backend, *indexPath, *schemaName)
		ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Printf("Error opening index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		fields, err := ix.DiscoverFields(ctx)
		if err != nil {
			fmt.Printf("Error discovering fields: %v\n", err)
			os.Exit(1)
		}

		if *format == "json" {
			jsonOut, _ := json.Marshal(fields)
			fmt.Println(string(jsonOut))
			return
		}

		for _, f := range fields {
			fmt.Printf("%s (%s):\n", f.Field, f.Type)
			fmt.Printf("  Documents: %d\n", f.DocCount)
			if f.Unique != nil {
				fmt.Printf("  Unique values: %d\n", *f.Unique)
			}
			if f.Weight != nil {
				fmt.Printf("  Weight: %.2f\n", *f.Weight)
			}
			if len(f.Examples) > 0 {
				fmt.Printf("  Examples: %v\n", f.Examples)
			}
		}

	case "values":
		fs := flag.NewFlagSet("discover values", flag.ExitOnError)
		indexPath := fs.String("i", "", "index path (required)")
		field := fs.String("field", "", "field name (required)")
		top := fs.Int("top", 20, "number of values to return")
		where := fs.String("w", "", "filter query")
		backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
		schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
		format := fs.String("format", "pretty", "output format: pretty or json")
		fs.Parse(args[1:])

		if *indexPath == "" || *field == "" {
			fs.Usage()
			os.Exit(1)
		}

		adapter := createAdapter(*backend, *indexPath, *schemaName)
		ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Printf("Error opening index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		values, err := ix.DiscoverValues(ctx, *field, *where, *top)
		if err != nil {
			fmt.Printf("Error discovering values: %v\n", err)
			os.Exit(1)
		}

		if *format == "json" {
			jsonOut, _ := json.Marshal(values)
			fmt.Println(string(jsonOut))
			return
		}

		fmt.Printf("Top values for field '%s':\n", *field)
		for _, v := range values {
			fmt.Printf("  %s: %d\n", v.Value, v.Count)
		}

	default:
		fmt.Printf("Unknown discover command: %s\n", subcmd)
		os.Exit(1)
	}
}

func handleStats(ctx context.Context, args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	indexPath := fs.String("i", "", "index path (required)")
	field := fs.String("field", "", "field name (required)")
	where := fs.String("w", "", "filter query")
	backend := fs.String("backend", "sqlite", "backend: sqlite or postgres")
	schemaName := fs.String("schema-name", "", "PostgreSQL schema name (default: ministore)")
	format := fs.String("format", "pretty", "output format: pretty or json")
	fs.Parse(args)

	if *indexPath == "" || *field == "" {
		fs.Usage()
		os.Exit(1)
	}

	adapter := createAdapter(*backend, *indexPath, *schemaName)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Printf("Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	stats, err := ix.Stats(ctx, *field, *where)
	if err != nil {
		fmt.Printf("Error getting stats: %v\n", err)
		os.Exit(1)
	}

	if *format == "json" {
		output := map[string]any{
			"field": stats.Field,
			"count": stats.Count,
		}
		if stats.Min != nil {
			output["min"] = *stats.Min
		}
		if stats.Max != nil {
			output["max"] = *stats.Max
		}
		if stats.Avg != nil {
			output["avg"] = *stats.Avg
		}
		if stats.Median != nil {
			output["median"] = *stats.Median
		}
		jsonOut, _ := json.Marshal(output)
		fmt.Println(string(jsonOut))
		return
	}

	fmt.Printf("Statistics for field '%s':\n", stats.Field)
	fmt.Printf("  Count: %d\n", stats.Count)
	if stats.Min != nil {
		fmt.Printf("  Min: %.2f\n", *stats.Min)
	}
	if stats.Max != nil {
		fmt.Printf("  Max: %.2f\n", *stats.Max)
	}
	if stats.Avg != nil {
		fmt.Printf("  Avg: %.2f\n", *stats.Avg)
	}
	if stats.Median != nil {
		fmt.Printf("  Median: %.2f\n", *stats.Median)
	}
}
