package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/sqlite"
	_ "modernc.org/sqlite"
)

func main() {
	indexPath := flag.String("index", "", "path to the SQLite index")
	query := flag.String("query", "", "query to execute")
	limit := flag.Int("limit", 5, "maximum results per search")
	iterations := flag.Int("iterations", 100, "measured search iterations")
	warmups := flag.Int("warmups", 5, "warmup search iterations")
	flag.Parse()

	if *indexPath == "" || *query == "" || *limit < 1 || *iterations < 1 || *warmups < 0 {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	ix, err := ministore.Open(ctx, sqlite.New(*indexPath), ministore.DefaultIndexOptions())
	if err != nil {
		fatal(err)
	}
	defer ix.Close()

	opts := ministore.SearchOptions{
		Limit:      *limit,
		CursorMode: ministore.CursorFull,
		Show:       ministore.OutputFieldSelector{Kind: ministore.ShowNone},
	}
	for range *warmups {
		if _, err := ix.Search(ctx, *query, opts); err != nil {
			fatal(err)
		}
	}

	started := time.Now()
	resultCount := 0
	for range *iterations {
		result, err := ix.Search(ctx, *query, opts)
		if err != nil {
			fatal(err)
		}
		resultCount += len(result.Items)
	}
	elapsed := time.Since(started)
	averageMS := float64(elapsed) / float64(time.Millisecond) / float64(*iterations)
	fmt.Printf("%.3f\t%d\n", averageMS, resultCount)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
