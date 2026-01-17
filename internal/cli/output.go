package cli

import (
	"encoding/json"
	"fmt"
	"io"
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
