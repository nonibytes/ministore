package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage"
	"github.com/ministore/ministore/ministore/storage/postgres"
	"github.com/ministore/ministore/ministore/storage/sqlite"
	_ "modernc.org/sqlite"
)

func main() {
	if len(os.Args) < 2 {
		printMainHelp()
		os.Exit(0)
	}

	cmd := os.Args[1]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		if len(os.Args) > 2 {
			printCommandHelp(os.Args[2])
		} else {
			printMainHelp()
		}
		os.Exit(0)
	}

	ctx := context.Background()
	args := os.Args[2:]

	switch cmd {
	case "index":
		handleIndex(ctx, args)
	case "put":
		handlePut(ctx, args)
	case "get":
		handleGet(ctx, args)
	case "peek":
		handlePeek(ctx, args)
	case "delete":
		handleDelete(ctx, args)
	case "search":
		handleSearch(ctx, args)
	case "discover":
		handleDiscover(ctx, args)
	case "stats":
		handleStats(ctx, args)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printMainHelp()
		os.Exit(1)
	}
}

func printMainHelp() {
	fmt.Println(`Search index (SQLite/PostgreSQL)

Usage: ministore <COMMAND>

Commands:
  index     Manage indexes: create, optimize, schema
  put       Insert/update docs (--path or --json JSONL)
  get       Get document by path (full JSON)
  peek      Get document metadata only
  delete    Delete by path or query
  search    Query documents (returns matches)
  discover  Explore field values
  stats     Compute min/max/avg for fields
  help      Print this message or the help of the given subcommand(s)

Options:
  -h, --help  Print help

Use ` + "`ministore <COMMAND> --help`" + ` for command details.`)
}

func printCommandHelp(cmd string) {
	switch cmd {
	case "index":
		printIndexHelp("")
	case "put":
		printPutHelp()
	case "get":
		printGetHelp()
	case "peek":
		printPeekHelp()
	case "delete":
		printDeleteHelp()
	case "search":
		printSearchHelp()
	case "discover":
		printDiscoverHelp("")
	case "stats":
		printStatsHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		os.Exit(1)
	}
}

func printIndexHelp(subcmd string) {
	if subcmd == "" {
		fmt.Println(`Manage indexes: create, optimize, schema

Usage: ministore index <COMMAND>

Commands:
  create    Create index (--schema file)
  schema    Show current schema
  optimize  Vacuum + rebuild FTS

Options:
  -h, --help  Print help`)
		return
	}

	switch subcmd {
	case "create":
		fmt.Println(`Create index (--schema file)

Usage: ministore index create [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index (SQLite file or PostgreSQL URL)
      --schema <SCHEMA>        Schema JSON file
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
	case "schema":
		fmt.Println(`Show current schema

Usage: ministore index schema [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
	case "optimize":
		fmt.Println(`Vacuum + rebuild FTS

Usage: ministore index optimize [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
	}
}

func printPutHelp() {
	fmt.Println(`Insert/update docs (--path or --json JSONL)

Usage: ministore put [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
  -p, --path <PATH>            Document path (for single doc mode)
      --set <SETS>             Set field: key=value (repeatable)
      --json                   Read JSONL from stdin (one JSON object per line)
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
}

func printGetHelp() {
	fmt.Println(`Get document by path (full JSON)

Usage: ministore get [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
  -p, --path <PATH>            Document path
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
}

func printPeekHelp() {
	fmt.Println(`Get document metadata only

Usage: ministore peek [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
  -p, --path <PATH>            Document path
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
}

func printDeleteHelp() {
	fmt.Println(`Delete by path or query

Usage: ministore delete [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
  -p, --path <PATH>            Document path (single delete)
  -w, --where <WHERE>          Query for batch delete
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
}

func printSearchHelp() {
	fmt.Println(`Query documents (returns matches)

Usage: ministore search [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
  -w, --where <WHERE>          Query (e.g. "category:rust priority>5")
      --limit <LIMIT>          Max results per page [default: 20]
      --after <AFTER>          Cursor for pagination
      --cursor <CURSOR>        Cursor mode: short|full [default: short]
      --rank <RANK>            Ranking: default|recency|none|field:<name> [default: default]
      --show <SHOW>            Fields: "all" or "f1,f2"
      --format <FORMAT>        Output: pretty|paths|json [default: pretty]
      --explain                Show query plan
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
}

func printDiscoverHelp(subcmd string) {
	if subcmd == "" {
		fmt.Println(`Explore field values

Usage: ministore discover <COMMAND>

Commands:
  fields    List all fields with stats
  values    List top values for a field

Options:
  -h, --help  Print help`)
		return
	}

	switch subcmd {
	case "fields":
		fmt.Println(`List all fields with stats

Usage: ministore discover fields [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
      --format <FORMAT>        Output: pretty|json [default: pretty]
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
	case "values":
		fmt.Println(`List top values for a field

Usage: ministore discover values [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
      --field <FIELD>          Field name
      --top <TOP>              Number of values [default: 20]
  -w, --where <WHERE>          Filter query
      --format <FORMAT>        Output: pretty|json [default: pretty]
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
	}
}

func printStatsHelp() {
	fmt.Println(`Compute min/max/avg for fields

Usage: ministore stats [OPTIONS]

Options:
  -i, --index <INDEX>          Path to index
      --field <FIELD>          Field name
  -w, --where <WHERE>          Filter query
      --format <FORMAT>        Output: pretty|json [default: pretty]
      --backend <BACKEND>      Backend: sqlite|postgres [default: sqlite]
      --schema-name <NAME>     PostgreSQL schema name [default: ministore]
  -h, --help                   Print help`)
}

// Argument parsing helpers
type args struct {
	args   []string
	values map[string]string
	flags  map[string]bool
	sets   []string
}

func parseArgs(input []string) *args {
	a := &args{
		args:   input,
		values: make(map[string]string),
		flags:  make(map[string]bool),
	}

	i := 0
	for i < len(input) {
		arg := input[i]
		if arg == "-h" || arg == "--help" {
			a.flags["help"] = true
			i++
			continue
		}

		if strings.HasPrefix(arg, "--") {
			key := strings.TrimPrefix(arg, "--")
			if key == "json" || key == "explain" {
				a.flags[key] = true
				i++
				continue
			}
			if key == "set" && i+1 < len(input) {
				a.sets = append(a.sets, input[i+1])
				i += 2
				continue
			}
			if i+1 < len(input) && !strings.HasPrefix(input[i+1], "-") {
				a.values[key] = input[i+1]
				i += 2
				continue
			}
			i++
			continue
		}

		if strings.HasPrefix(arg, "-") && len(arg) == 2 {
			key := string(arg[1])
			if i+1 < len(input) && !strings.HasPrefix(input[i+1], "-") {
				a.values[key] = input[i+1]
				i += 2
				continue
			}
		}
		i++
	}

	return a
}

func (a *args) get(keys ...string) string {
	for _, k := range keys {
		if v, ok := a.values[k]; ok {
			return v
		}
	}
	return ""
}

func (a *args) getInt(keys ...string) int {
	s := a.get(keys...)
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

func (a *args) has(key string) bool {
	return a.flags[key]
}

func (a *args) require(name string, keys ...string) string {
	v := a.get(keys...)
	if v == "" {
		fmt.Fprintf(os.Stderr, "Error: --%s is required\n", name)
		os.Exit(1)
	}
	return v
}

// Adapter creation
func createAdapter(a *args) storage.Adapter {
	backend := a.get("backend")
	indexPath := a.get("i", "index")
	schemaName := a.get("schema-name")

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

// Command handlers
func handleIndex(ctx context.Context, cmdArgs []string) {
	if len(cmdArgs) == 0 || cmdArgs[0] == "-h" || cmdArgs[0] == "--help" || cmdArgs[0] == "help" {
		if len(cmdArgs) > 1 {
			printIndexHelp(cmdArgs[1])
		} else {
			printIndexHelp("")
		}
		return
	}

	subcmd := cmdArgs[0]
	a := parseArgs(cmdArgs[1:])

	if a.has("help") {
		printIndexHelp(subcmd)
		return
	}

	switch subcmd {
	case "create":
		indexPath := a.require("index", "i", "index")
		schemaFile := a.require("schema", "schema")

		schemaData, err := os.ReadFile(schemaFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading schema file: %v\n", err)
			os.Exit(1)
		}

		var schema ministore.Schema
		if err := json.Unmarshal(schemaData, &schema); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing schema: %v\n", err)
			os.Exit(1)
		}

		a.values["index"] = indexPath
		adapter := createAdapter(a)
		ix, err := ministore.Create(ctx, adapter, schema, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		fmt.Printf("Created index: %s\n", indexPath)
		fmt.Printf("Fields: %d\n", len(schema.Fields))

	case "schema":
		a.require("index", "i", "index")
		adapter := createAdapter(a)
		ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		schema := ix.Schema()
		schemaJSON, _ := json.MarshalIndent(schema, "", "  ")
		fmt.Println(string(schemaJSON))

	case "optimize":
		a.require("index", "i", "index")
		adapter := createAdapter(a)
		ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
			os.Exit(1)
		}
		defer ix.Close()

		if err := ix.Optimize(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error optimizing: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Index optimized")

	default:
		fmt.Fprintf(os.Stderr, "Unknown index command: %s\n", subcmd)
		printIndexHelp("")
		os.Exit(1)
	}
}

func handlePut(ctx context.Context, cmdArgs []string) {
	a := parseArgs(cmdArgs)
	if a.has("help") {
		printPutHelp()
		return
	}

	a.require("index", "i", "index")
	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	if a.has("json") {
		scanner := bufio.NewScanner(os.Stdin)
		count := 0
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if err := ix.PutJSON(ctx, []byte(line)); err != nil {
				fmt.Fprintf(os.Stderr, "Error putting item: %v\n", err)
				os.Exit(1)
			}
			count++
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Put %d items\n", count)
	} else {
		path := a.require("path", "p", "path")
		doc := map[string]any{"path": path}
		for _, kv := range a.sets {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) == 2 {
				doc[parts[0]] = parts[1]
			}
		}

		docJSON, _ := json.Marshal(doc)
		if err := ix.PutJSON(ctx, docJSON); err != nil {
			fmt.Fprintf(os.Stderr, "Error putting item: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Put: %s\n", path)
	}
}

func handleGet(ctx context.Context, cmdArgs []string) {
	a := parseArgs(cmdArgs)
	if a.has("help") {
		printGetHelp()
		return
	}

	a.require("index", "i", "index")
	path := a.require("path", "p", "path")

	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	item, err := ix.Get(ctx, path)
	if err != nil {
		if ministore.IsKind(err, ministore.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Not found: %s\n", path)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Path: %s\n", item.Path)
	fmt.Printf("Created: %d\n", item.Meta.CreatedAtMS)
	fmt.Printf("Updated: %d\n", item.Meta.UpdatedAtMS)
	fmt.Printf("\n%s\n", string(item.DocJSON))
}

func handlePeek(ctx context.Context, cmdArgs []string) {
	a := parseArgs(cmdArgs)
	if a.has("help") {
		printPeekHelp()
		return
	}

	a.require("index", "i", "index")
	path := a.require("path", "p", "path")

	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	data, err := ix.Peek(ctx, path)
	if err != nil {
		if ministore.IsKind(err, ministore.ErrNotFound) {
			fmt.Fprintf(os.Stderr, "Not found: %s\n", path)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var obj any
	if json.Unmarshal(data, &obj) == nil {
		pretty, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(pretty))
	} else {
		fmt.Println(string(data))
	}
}

func handleDelete(ctx context.Context, cmdArgs []string) {
	a := parseArgs(cmdArgs)
	if a.has("help") {
		printDeleteHelp()
		return
	}

	a.require("index", "i", "index")
	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	path := a.get("p", "path")
	where := a.get("w", "where")

	if path == "" && where == "" {
		fmt.Fprintln(os.Stderr, "Error: --path or --where required")
		os.Exit(1)
	}

	if path != "" {
		deleted, err := ix.Delete(ctx, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if deleted {
			fmt.Printf("Deleted: %s\n", path)
		} else {
			fmt.Printf("Not found: %s\n", path)
		}
	} else {
		count, err := ix.DeleteWhere(ctx, where)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Deleted %d items\n", count)
	}
}

func handleSearch(ctx context.Context, cmdArgs []string) {
	a := parseArgs(cmdArgs)
	if a.has("help") {
		printSearchHelp()
		return
	}

	a.require("index", "i", "index")
	where := a.require("where", "w", "where")

	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	opts := ministore.SearchOptions{
		Limit:   20,
		After:   a.get("after"),
		Explain: a.has("explain"),
	}

	if limit := a.getInt("limit"); limit > 0 {
		opts.Limit = limit
	}

	// Parse show
	show := a.get("show")
	switch show {
	case "", "none":
		opts.Show.Kind = ministore.ShowNone
	case "all":
		opts.Show.Kind = ministore.ShowAll
	default:
		opts.Show.Kind = ministore.ShowFields
		opts.Show.Fields = strings.Split(show, ",")
	}

	// Parse rank
	rank := a.get("rank")
	switch {
	case rank == "" || rank == "default":
		opts.Rank.Kind = ministore.RankDefault
	case rank == "recency":
		opts.Rank.Kind = ministore.RankRecency
	case rank == "none":
		opts.Rank.Kind = ministore.RankNone
	case strings.HasPrefix(rank, "field:"):
		opts.Rank.Kind = ministore.RankField
		opts.Rank.Field = strings.TrimPrefix(rank, "field:")
	}

	result, err := ix.Search(ctx, where, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	format := a.get("format")
	if format == "json" {
		output := map[string]any{
			"items":    make([]any, 0, len(result.Items)),
			"has_more": result.HasMore,
		}
		if result.NextCursor != "" {
			output["next_cursor"] = result.NextCursor
		}
		for _, item := range result.Items {
			var obj any
			if json.Unmarshal(item, &obj) == nil {
				output["items"] = append(output["items"].([]any), obj)
			}
		}
		jsonOut, _ := json.Marshal(output)
		fmt.Println(string(jsonOut))
		return
	}

	// Pretty format
	if opts.Explain {
		fmt.Println("=== Query Plan ===")
		for _, step := range result.ExplainSteps {
			fmt.Printf("  %s\n", step)
		}
		fmt.Println("\n=== SQL ===")
		fmt.Println(result.ExplainSQL)
		fmt.Println("\n=== Results ===")
	}

	for _, item := range result.Items {
		var obj any
		if json.Unmarshal(item, &obj) == nil {
			pretty, _ := json.MarshalIndent(obj, "", "  ")
			fmt.Println(string(pretty))
		} else {
			fmt.Println(string(item))
		}
	}

	fmt.Printf("\n--- %d results", len(result.Items))
	if result.HasMore {
		fmt.Print(", more available")
		if result.NextCursor != "" {
			fmt.Printf(" (cursor: %s)", result.NextCursor)
		}
	}
	fmt.Println(" ---")
}

func handleDiscover(ctx context.Context, cmdArgs []string) {
	if len(cmdArgs) == 0 || cmdArgs[0] == "-h" || cmdArgs[0] == "--help" || cmdArgs[0] == "help" {
		if len(cmdArgs) > 1 {
			printDiscoverHelp(cmdArgs[1])
		} else {
			printDiscoverHelp("")
		}
		return
	}

	subcmd := cmdArgs[0]
	a := parseArgs(cmdArgs[1:])

	if a.has("help") {
		printDiscoverHelp(subcmd)
		return
	}

	a.require("index", "i", "index")
	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	format := a.get("format")

	switch subcmd {
	case "fields":
		fields, err := ix.DiscoverFields(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if format == "json" {
			jsonOut, _ := json.Marshal(fields)
			fmt.Println(string(jsonOut))
			return
		}

		for _, f := range fields {
			fmt.Printf("%s (%s):\n", f.Field, f.Type)
			fmt.Printf("  Documents: %d\n", f.DocCount)
			if f.Unique != nil {
				fmt.Printf("  Unique: %d\n", *f.Unique)
			}
			if f.Weight != nil {
				fmt.Printf("  Weight: %.2f\n", *f.Weight)
			}
			if len(f.Examples) > 0 {
				fmt.Printf("  Examples: %v\n", f.Examples)
			}
		}

	case "values":
		field := a.require("field", "field")
		where := a.get("w", "where")
		top := a.getInt("top")
		if top == 0 {
			top = 20
		}

		values, err := ix.DiscoverValues(ctx, field, where, top)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		if format == "json" {
			jsonOut, _ := json.Marshal(values)
			fmt.Println(string(jsonOut))
			return
		}

		fmt.Printf("Top values for '%s':\n", field)
		for _, v := range values {
			fmt.Printf("  %s: %d\n", v.Value, v.Count)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown discover command: %s\n", subcmd)
		printDiscoverHelp("")
		os.Exit(1)
	}
}

func handleStats(ctx context.Context, cmdArgs []string) {
	a := parseArgs(cmdArgs)
	if a.has("help") {
		printStatsHelp()
		return
	}

	a.require("index", "i", "index")
	field := a.require("field", "field")
	where := a.get("w", "where")

	adapter := createAdapter(a)
	ix, err := ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening index: %v\n", err)
		os.Exit(1)
	}
	defer ix.Close()

	stats, err := ix.Stats(ctx, field, where)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	format := a.get("format")
	if format == "json" {
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

	fmt.Printf("Statistics for '%s':\n", stats.Field)
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
