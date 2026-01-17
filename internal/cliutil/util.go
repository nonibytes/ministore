package cliutil

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/nonibytes/ministore/internal/cliopt"
)

type OutputFormat string

const (
	FormatPretty OutputFormat = "pretty"
	FormatPaths  OutputFormat = "paths"
	FormatJSON   OutputFormat = "json"
)

func ParseOutputFormat(s string) OutputFormat {
	switch OutputFormat(s) {
	case FormatPretty, FormatPaths, FormatJSON:
		return OutputFormat(s)
	default:
		return FormatPretty
	}
}

func PrintJSON(w io.Writer, v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(w, string(b))
}

// ResolveIndexRef transforms the user-provided -i/--index value into a backend-specific reference.
func ResolveIndexRef(g cliopt.GlobalOptions, index string) string {
	switch strings.ToLower(g.Backend) {
	case "sqlite":
		if strings.Contains(index, string(filepath.Separator)) || strings.HasSuffix(index, ".db") {
			return index
		}
		return filepath.Join(g.SQLitePath, index+".db")
	default:
		return index
	}
}
