package ministore

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func sampleSearchPage() SearchResultPage {
	return SearchResultPage{
		Items: [][]byte{
			[]byte(`{"title":"Fast search","path":"/guides/search","priority":10,"category":"guide"}`),
			[]byte(`{"path":"/notes/sqlite","title":"SQLite notes"}`),
		},
		NextCursor: "c:next",
		HasMore:    true,
	}
}

func TestFormatSearchResultsPretty(t *testing.T) {
	elapsed := 7 * time.Millisecond
	formatted, err := FormatSearchResults(sampleSearchPage(), SearchOutputOptions{
		Format:  SearchOutputPretty,
		Elapsed: &elapsed,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "Found 2 items in 7ms\n" +
		"- /guides/search\n" +
		"  category: guide\n" +
		"  priority: 10\n" +
		"  title: Fast search\n" +
		"- /notes/sqlite\n" +
		"  title: SQLite notes\n" +
		"\nNext cursor: c:next\n"
	if formatted != want {
		t.Fatalf("unexpected pretty output:\n%s", formatted)
	}
}

func TestFormatSearchResultsPaths(t *testing.T) {
	formatted, err := FormatSearchResults(sampleSearchPage(), SearchOutputOptions{Format: SearchOutputPaths})
	if err != nil {
		t.Fatal(err)
	}
	if formatted != "/guides/search\n/notes/sqlite\n" {
		t.Fatalf("unexpected paths output: %q", formatted)
	}
}

func TestFormatSearchResultsPrettyUsesSingularItem(t *testing.T) {
	page := SearchResultPage{Items: [][]byte{[]byte(`{"path":"/one"}`)}}
	formatted, err := FormatSearchResults(page, SearchOutputOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if formatted != "Found 1 item\n- /one\n" {
		t.Fatalf("unexpected singular output: %q", formatted)
	}
}

func TestFormatSearchResultsJSON(t *testing.T) {
	formatted, err := FormatSearchResults(sampleSearchPage(), SearchOutputOptions{Format: SearchOutputJSON})
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Items      []json.RawMessage `json:"items"`
		NextCursor string            `json:"next_cursor"`
		HasMore    bool              `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(formatted), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Items) != 2 || envelope.NextCursor != "c:next" || !envelope.HasMore {
		t.Fatalf("unexpected JSON envelope: %+v", envelope)
	}
}

func TestFormatSearchResultsRejectsInvalidInput(t *testing.T) {
	if _, err := FormatSearchResults(SearchResultPage{}, SearchOutputOptions{Format: "yaml"}); err == nil {
		t.Fatal("expected invalid format error")
	}
	page := SearchResultPage{Items: [][]byte{[]byte(`not-json`)}}
	if _, err := FormatSearchResults(page, SearchOutputOptions{}); err == nil || !strings.Contains(err.Error(), "invalid JSON object") {
		t.Fatalf("expected invalid item error, got %v", err)
	}
}
