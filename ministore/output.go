package ministore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// SearchOutputFormat selects the representation produced by FormatSearchResults.
type SearchOutputFormat string

const (
	SearchOutputPretty SearchOutputFormat = "pretty"
	SearchOutputPaths  SearchOutputFormat = "paths"
	SearchOutputJSON   SearchOutputFormat = "json"
)

// SearchOutputOptions configures search-result formatting.
type SearchOutputOptions struct {
	Format  SearchOutputFormat
	Elapsed *time.Duration
}

// ParseSearchOutputFormat validates a search output format. An empty format
// selects the human-readable pretty format.
func ParseSearchOutputFormat(value string) (SearchOutputFormat, error) {
	switch SearchOutputFormat(strings.ToLower(value)) {
	case "", SearchOutputPretty:
		return SearchOutputPretty, nil
	case SearchOutputPaths:
		return SearchOutputPaths, nil
	case SearchOutputJSON:
		return SearchOutputJSON, nil
	default:
		return "", fmt.Errorf("unknown search output format %q; expected pretty, paths, or json", value)
	}
}

// FormatSearchResults renders a search page for terminal or machine output.
// Pretty and paths are human-readable; JSON has a stable page envelope.
func FormatSearchResults(page SearchResultPage, options SearchOutputOptions) (string, error) {
	format, err := ParseSearchOutputFormat(string(options.Format))
	if err != nil {
		return "", err
	}

	switch format {
	case SearchOutputPretty:
		return formatSearchPretty(page, options.Elapsed)
	case SearchOutputPaths:
		return formatSearchPaths(page)
	case SearchOutputJSON:
		return formatSearchJSON(page)
	default:
		panic("unreachable search output format")
	}
}

func formatSearchPretty(page SearchResultPage, elapsed *time.Duration) (string, error) {
	var output strings.Builder
	fmt.Fprintf(&output, "Found %d items", len(page.Items))
	if elapsed != nil {
		fmt.Fprintf(&output, " in %dms", elapsed.Milliseconds())
	}
	output.WriteByte('\n')

	for index, rawItem := range page.Items {
		item, err := decodeSearchItem(rawItem, index)
		if err != nil {
			return "", err
		}
		path := "(no path)"
		if rawPath, ok := item["path"]; ok {
			if err := json.Unmarshal(rawPath, &path); err != nil {
				return "", fmt.Errorf("format search result %d: path must be a string: %w", index, err)
			}
		}
		fmt.Fprintf(&output, "- %s\n", path)

		keys := make([]string, 0, len(item))
		for key := range item {
			if key != "path" {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			value, err := formatHumanJSONValue(item[key])
			if err != nil {
				return "", fmt.Errorf("format search result %d field %q: %w", index, key, err)
			}
			fmt.Fprintf(&output, "  %s: %s\n", key, value)
		}
	}

	if page.NextCursor != "" {
		fmt.Fprintf(&output, "\nNext cursor: %s\n", page.NextCursor)
	} else if page.HasMore {
		output.WriteString("\nMore results available\n")
	}
	if len(page.ExplainSteps) > 0 {
		output.WriteString("\nExplanation:\n")
		for _, step := range page.ExplainSteps {
			fmt.Fprintf(&output, "  %s\n", step)
		}
	}
	if page.ExplainSQL != "" {
		fmt.Fprintf(&output, "\nSQL: %s\n", page.ExplainSQL)
	}

	return output.String(), nil
}

func formatSearchPaths(page SearchResultPage) (string, error) {
	var output strings.Builder
	for index, rawItem := range page.Items {
		item, err := decodeSearchItem(rawItem, index)
		if err != nil {
			return "", err
		}
		rawPath, ok := item["path"]
		if !ok {
			continue
		}
		var path string
		if err := json.Unmarshal(rawPath, &path); err != nil {
			return "", fmt.Errorf("format search result %d: path must be a string: %w", index, err)
		}
		output.WriteString(path)
		output.WriteByte('\n')
	}
	return output.String(), nil
}

func formatSearchJSON(page SearchResultPage) (string, error) {
	envelope := struct {
		Items        []json.RawMessage `json:"items"`
		NextCursor   string            `json:"next_cursor,omitempty"`
		HasMore      bool              `json:"has_more"`
		ExplainSQL   string            `json:"explain_sql,omitempty"`
		ExplainSteps []string          `json:"explain_steps,omitempty"`
	}{
		Items:        make([]json.RawMessage, len(page.Items)),
		NextCursor:   page.NextCursor,
		HasMore:      page.HasMore,
		ExplainSQL:   page.ExplainSQL,
		ExplainSteps: page.ExplainSteps,
	}
	for index, item := range page.Items {
		envelope.Items[index] = json.RawMessage(item)
	}
	encoded, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return "", fmt.Errorf("format search results as JSON: %w", err)
	}
	return string(encoded) + "\n", nil
}

func decodeSearchItem(raw []byte, index int) (map[string]json.RawMessage, error) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, fmt.Errorf("format search result %d: invalid JSON object: %w", index, err)
	}
	if item == nil {
		return nil, fmt.Errorf("format search result %d: expected JSON object", index)
	}
	return item, nil
}

func formatHumanJSONValue(raw json.RawMessage) (string, error) {
	if len(raw) > 0 && raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return value, nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return "", err
	}
	return compact.String(), nil
}
