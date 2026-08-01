package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/postgres"
	ministoreokf "github.com/ministore/ministore/okf"
)

func printOKFHelp() {
	fmt.Println(`Validate and synchronize OKF bundles

Usage: ministore okf <COMMAND>

Commands:
  validate  Validate a bundle without opening an index
  sync      Atomically synchronize a bundle into an OKF index

Validate: ministore okf validate --bundle DIR [--strict] [--format pretty|json]
Sync:     ministore okf sync --bundle DIR --index INDEX [--strict] [--dry-run] [--format pretty|json]`)
}

func handleOKF(ctx context.Context, argv []string) {
	if len(argv) == 0 || argv[0] == "--help" {
		printOKFHelp()
		return
	}
	sub := argv[0]
	a := parseArgs(argv[1:])
	bundle := a.get("bundle")
	if bundle == "" {
		fatalOKF("--bundle is required")
	}
	switch sub {
	case "validate":
		validateOKF(ctx, bundle, a)
	case "sync":
		syncOKF(ctx, bundle, a)
	default:
		fatalOKF("unknown okf command: " + sub)
	}
}

func validateOKF(ctx context.Context, bundle string, a *args) {
	jsonOutput := a.get("format") == "json"
	var spool *os.File
	var err error
	if jsonOutput {
		spool, err = os.CreateTemp("", "ministore-okf-findings-")
		if err != nil {
			fatalOKF(err.Error())
		}
		defer func() { name := spool.Name(); _ = spool.Close(); _ = os.Remove(name) }()
	}
	summary, err := ministoreokf.ValidateBundle(ctx, bundle, ministoreokf.ValidateOptions{}, func(f ministoreokf.Finding) error {
		if jsonOutput {
			return json.NewEncoder(spool).Encode(f)
		}
		encoded, _ := json.Marshal(f)
		fmt.Println(string(encoded))
		return nil
	})
	if err != nil {
		fatalOKF(err.Error())
	}
	ok := summary.Errors == 0 && (!a.has("strict") || summary.Warnings == 0)
	if jsonOutput {
		if _, err = spool.Seek(0, 0); err != nil {
			fatalOKF(err.Error())
		}
		summaryJSON, _ := json.Marshal(summary)
		fmt.Print("{\"findings\":[")
		reader := bufio.NewReader(spool)
		first := true
		for {
			line, readErr := reader.ReadString('\n')
			if len(line) > 0 {
				if !first {
					fmt.Print(",")
				}
				first = false
				fmt.Print(strings.TrimSuffix(line, "\n"))
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				fatalOKF(readErr.Error())
			}
		}
		fmt.Printf("],\"ok\":%t,\"validation\":%s}\n", ok, summaryJSON)
	} else {
		outputOKF("pretty", map[string]any{"ok": ok, "validation": summary})
	}
	if !ok {
		os.Exit(1)
	}
}

func syncOKF(ctx context.Context, bundle string, a *args) {
	if a.indexPath() == "" {
		fatalOKF("--index is required")
	}
	dry := a.has("dry-run")
	adapter := createAdapter(a)
	backend := a.get("backend")
	sqliteBackend := backend == "" || backend == "sqlite"
	targetExists := true
	if sqliteBackend {
		if _, statErr := os.Stat(a.indexPath()); statErr != nil {
			if !os.IsNotExist(statErr) {
				fatalOKF(statErr.Error())
			}
			targetExists = false
		}
	}
	if !sqliteBackend {
		empty, stateErr := adapter.(*postgres.Adapter).SchemaEmpty(ctx)
		if stateErr != nil {
			fatalOKF(stateErr.Error())
		}
		targetExists = !empty
	}
	var ix *ministore.Index
	var err error
	if dry && targetExists {
		ix, err = ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	}
	if dry && (!targetExists || err != nil) {
		directory, e := os.MkdirTemp("", "ministore-okf-dry-")
		if e != nil {
			fatalOKF(e.Error())
		}
		defer os.RemoveAll(directory)
		tempArgs := *a
		tempArgs.values = map[string]string{"index": filepath.Join(directory, "index.db")}
		ix, err = ministore.Create(ctx, createAdapter(&tempArgs), ministoreokf.ProjectionSchema(), ministore.DefaultIndexOptions())
	} else if !dry && targetExists {
		ix, err = ministore.Open(ctx, adapter, ministore.DefaultIndexOptions())
	} else if !dry {
		err = fmt.Errorf("target index does not exist")
	}
	if !dry && err != nil && (!sqliteBackend || !targetExists) {
		var summary ministoreokf.ValidationSummary
		summary, err = ministoreokf.ValidateBundle(ctx, bundle, ministoreokf.ValidateOptions{}, nil)
		if err == nil && summary.Errors == 0 && (!a.has("strict") || summary.Warnings == 0) {
			ix, err = ministore.Create(ctx, adapter, ministoreokf.ProjectionSchema(), ministore.DefaultIndexOptions())
		}
	}
	if err != nil {
		fatalOKF(err.Error())
	}
	defer ix.Close()
	report, err := ministoreokf.Sync(ctx, bundle, ix, ministoreokf.SyncOptions{Strict: a.has("strict"), DryRun: dry})
	if err != nil {
		fatalOKF(err.Error())
	}
	outputOKF(a.get("format"), report)
	if !report.OK {
		os.Exit(1)
	}
}

func outputOKF(format string, value any) {
	if format == "json" {
		encoded, _ := json.Marshal(value)
		fmt.Println(string(encoded))
		return
	}
	encoded, _ := json.MarshalIndent(value, "", "  ")
	fmt.Println(string(encoded))
}
func fatalOKF(message string) { fmt.Fprintln(os.Stderr, "error:", message); os.Exit(1) }
