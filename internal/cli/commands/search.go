package commands

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
	"github.com/nonibytes/ministore/pkg/ministore/index"
)

func RunSearch(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName, where, after, cursorMode, rank, show, format string
	var limit int
	var explain bool
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&where, "where", "", "where query")
	fs.StringVar(&where, "w", "", "where query")
	fs.IntVar(&limit, "limit", 20, "limit")
	fs.StringVar(&after, "after", "", "cursor")
	fs.StringVar(&cursorMode, "cursor", "short", "cursor: short|full")
	fs.StringVar(&rank, "rank", "default", "rank: default|recency|none|field:<name>")
	fs.StringVar(&show, "show", "", "show: all|f1,f2")
	fs.StringVar(&format, "format", "pretty", "format: pretty|paths|json")
	fs.BoolVar(&explain, "explain", false, "explain")
	if err := fs.Parse(argv); err != nil {
		return 2
	}
	if indexName == "" || where == "" {
		fmt.Fprintln(os.Stderr, "missing --index or --where")
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

	opts := index.SearchOptions{
		Rank:       index.ParseRankMode(rank),
		Limit:      limit,
		After:      after,
		CursorMode: index.ParseCursorMode(cursorMode),
		Show:       index.ParseShow(show),
		Explain:    explain,
	}

	start := time.Now()
	page, err := idx.Search(ctx, where, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	elapsed := time.Since(start)
	commandsPrintSearch(cliutil.ParseOutputFormat(format), page, elapsed)
	return 0
}

func commandsPrintSearch(fmtOut cliutil.OutputFormat, page index.SearchResultPage, dur time.Duration) {
	switch fmtOut {
	case cliutil.FormatJSON:
		cliutil.PrintJSON(os.Stdout, page)
	case cliutil.FormatPaths:
		for _, it := range page.Items {
			if p, ok := it["path"].(string); ok {
				fmt.Fprintln(os.Stdout, p)
			}
		}
	default:
		fmt.Fprintf(os.Stdout, "Found %d items in %dms\n", len(page.Items), dur.Milliseconds())
		for _, it := range page.Items {
			if p, ok := it["path"].(string); ok {
				fmt.Fprintf(os.Stdout, "- %s\n", p)
			} else {
				fmt.Fprintln(os.Stdout, "- (no path)")
			}
		}
		if page.NextCursor != "" {
			fmt.Fprintf(os.Stdout, "\nnext: %s\n", page.NextCursor)
		}
		if len(page.ExplainPlan) > 0 {
			fmt.Fprintln(os.Stdout, "\nExplanation:")
			for _, s := range page.ExplainPlan {
				fmt.Fprintf(os.Stdout, "  %s\n", s)
			}
		}
		if page.ExplainQuery != "" {
			fmt.Fprintf(os.Stdout, "\nQuery:\n%s\n", page.ExplainQuery)
		}
	}
}
