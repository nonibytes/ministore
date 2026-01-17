package commands

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/internal/cliutil"
	"github.com/nonibytes/ministore/pkg/ministore"
	"github.com/nonibytes/ministore/pkg/ministore/index"
)

func RunPut(g cliopt.GlobalOptions, argv []string) int {
	fs := flag.NewFlagSet("put", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var indexName string
	var path string
	var jsonStdin bool
	var importPath string
	var sets multiString
	fs.StringVar(&indexName, "index", "", "index")
	fs.StringVar(&indexName, "i", "", "index")
	fs.StringVar(&path, "path", "", "path")
	fs.StringVar(&path, "p", "", "path")
	fs.BoolVar(&jsonStdin, "json", false, "read JSON lines from stdin")
	fs.StringVar(&importPath, "import", "", "import JSONL file")
	fs.Var(&sets, "set", "set k=v (repeatable)")
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

	// single doc mode
	if path != "" {
		doc := map[string]any{"path": path}
		for _, kv := range sets {
			k, v, ok := splitOnce(kv, '=')
			if !ok {
				doc[kv] = true
				continue
			}
			doc[k] = v
		}
		if err := idx.Put(ctx, doc); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Fprintln(os.Stdout, "put")
		return 0
	}

	// import/jsonl mode
	var r *os.File
	if importPath != "" {
		f, err := os.Open(importPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		r = f
		defer f.Close()
	} else if jsonStdin {
		r = os.Stdin
	} else {
		fmt.Fprintln(os.Stderr, "provide --path or --json or --import")
		return 2
	}

	batch := index.NewBatch()
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(bytesTrimSpace(line)) == 0 {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal(line, &doc); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if err := batch.Put(doc); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	count, err := idx.Batch(ctx, batch)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "imported %d\n", count)
	return 0
}

// ---- helpers (local to commands package) ----

type multiString []string

func (m *multiString) String() string { return "" }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func splitOnce(s string, sep byte) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			return s[:i], s[i+1:], true
		}
	}
	return "", "", false
}

func bytesTrimSpace(b []byte) []byte {
	i := 0
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\r' || b[i] == '\t') {
		i++
	}
	j := len(b)
	for j > i && (b[j-1] == ' ' || b[j-1] == '\n' || b[j-1] == '\r' || b[j-1] == '\t') {
		j--
	}
	return b[i:j]
}
