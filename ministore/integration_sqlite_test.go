package ministore_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/sqlite"
	_ "modernc.org/sqlite"
)

func monotonicNow(start time.Time) func() time.Time {
	var mu sync.Mutex
	t := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t = t.Add(time.Millisecond)
		return t
	}
}

func newIndex(t *testing.T, schema ministore.Schema) (*ministore.Index, string) {
	t.Helper()

	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	opts := ministore.DefaultIndexOptions()
	opts.Now = monotonicNow(time.Unix(1700000000, 0)) // deterministic ordering

	ix, err := ministore.Create(context.Background(), sqlite.New(dbPath), schema, opts)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = ix.Close() })
	return ix, dbPath
}

func pathsFromItems(t *testing.T, items [][]byte) []string {
	t.Helper()
	var out []string
	for _, b := range items {
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal item: %v; item=%s", err, string(b))
		}
		p, _ := m["path"].(string)
		out = append(out, p)
	}
	return out
}

func TestPutGetDelete_SQLite(t *testing.T) {
	schema := ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"tags":     {Type: ministore.FieldKeyword, Multi: true},
			"priority": {Type: ministore.FieldNumber},
			"done":     {Type: ministore.FieldBool},
			"due":      {Type: ministore.FieldDate},
		},
	}
	ix, _ := newIndex(t, schema)
	ctx := context.Background()

	doc := map[string]any{
		"path":     "/a",
		"tags":     []any{"work", "urgent"},
		"priority": 7,
		"done":     false,
		"due":      "2025-01-02",
	}
	b, _ := json.Marshal(doc)
	if err := ix.PutJSON(ctx, b); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}

	got, err := ix.Get(ctx, "/a")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Path != "/a" {
		t.Fatalf("unexpected path: %s", got.Path)
	}
	if got.Meta.CreatedAtMS == 0 || got.Meta.UpdatedAtMS == 0 {
		t.Fatalf("expected timestamps, got created=%d updated=%d", got.Meta.CreatedAtMS, got.Meta.UpdatedAtMS)
	}

	deleted, err := ix.Delete(ctx, "/a")
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Fatalf("expected deleted=true")
	}

	_, err = ix.Get(ctx, "/a")
	if err == nil || !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("expected not found, got: %v", err)
	}
}

func TestSearchFiltersAndPagination_Recency_SQLite(t *testing.T) {
	schema := ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"tags":     {Type: ministore.FieldKeyword, Multi: true},
			"priority": {Type: ministore.FieldNumber},
			"done":     {Type: ministore.FieldBool},
			"due":      {Type: ministore.FieldDate},
		},
	}
	ix, _ := newIndex(t, schema)
	ctx := context.Background()

	put := func(path string, tags []string, pr int, done bool, due string) {
		t.Helper()
		doc := map[string]any{
			"path":     path,
			"tags":     tags,
			"priority": pr,
			"done":     done,
			"due":      due,
		}
		b, _ := json.Marshal(doc)
		if err := ix.PutJSON(ctx, b); err != nil {
			t.Fatalf("PutJSON(%s): %v", path, err)
		}
	}

	put("/1", []string{"work"}, 3, false, "2025-01-01")
	put("/2", []string{"work", "urgent"}, 10, true, "2025-02-01")
	put("/3", []string{"home"}, 7, false, "2024-12-31")

	// Keyword exact
	{
		res, err := ix.Search(ctx, "tags:work", ministore.SearchOptions{
			Rank:  ministore.RankMode{Kind: ministore.RankRecency},
			Limit: 10,
			Show:  ministore.OutputFieldSelector{Kind: ministore.ShowNone},
		})
		if err != nil {
			t.Fatalf("Search tags:work: %v", err)
		}
		got := pathsFromItems(t, res.Items)
		sort.Strings(got)
		want := []string{"/1", "/2"}
		if len(got) != len(want) {
			t.Fatalf("got %v want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	}

	// Number comparison
	{
		res, err := ix.Search(ctx, "priority>5", ministore.SearchOptions{
			Rank:  ministore.RankMode{Kind: ministore.RankRecency},
			Limit: 10,
			Show:  ministore.OutputFieldSelector{Kind: ministore.ShowNone},
		})
		if err != nil {
			t.Fatalf("Search priority>5: %v", err)
		}
		got := pathsFromItems(t, res.Items)
		sort.Strings(got)
		want := []string{"/2", "/3"}
		if len(got) != len(want) {
			t.Fatalf("got %v want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	}

	// Bool via field:false coercion
	{
		res, err := ix.Search(ctx, "done:false", ministore.SearchOptions{
			Rank:  ministore.RankMode{Kind: ministore.RankRecency},
			Limit: 10,
			Show:  ministore.OutputFieldSelector{Kind: ministore.ShowNone},
		})
		if err != nil {
			t.Fatalf("Search done:false: %v", err)
		}
		got := pathsFromItems(t, res.Items)
		sort.Strings(got)
		want := []string{"/1", "/3"}
		if len(got) != len(want) {
			t.Fatalf("got %v want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	}

	// Date comparison (dates must be quoted in queries)
	{
		res, err := ix.Search(ctx, `due>"2025-01-15"`, ministore.SearchOptions{
			Rank:  ministore.RankMode{Kind: ministore.RankRecency},
			Limit: 10,
			Show:  ministore.OutputFieldSelector{Kind: ministore.ShowNone},
		})
		if err != nil {
			t.Fatalf("Search due>...: %v", err)
		}
		got := pathsFromItems(t, res.Items)
		if len(got) != 1 || got[0] != "/2" {
			t.Fatalf("got %v want [/2]", got)
		}
	}

	// Cursor pagination (recency): limit=1, page through tags:work
	{
		opts := ministore.SearchOptions{
			Rank:       ministore.RankMode{Kind: ministore.RankRecency},
			Limit:      1,
			CursorMode: ministore.CursorFull,
			Show:       ministore.OutputFieldSelector{Kind: ministore.ShowNone},
		}
		page1, err := ix.Search(ctx, "tags:work", opts)
		if err != nil {
			t.Fatalf("page1: %v", err)
		}
		if len(page1.Items) != 1 || !page1.HasMore || page1.NextCursor == "" {
			t.Fatalf("expected 1 item + hasMore + cursor; got len=%d hasMore=%v cursor=%q", len(page1.Items), page1.HasMore, page1.NextCursor)
		}
		p1 := pathsFromItems(t, page1.Items)[0]

		opts.After = page1.NextCursor
		page2, err := ix.Search(ctx, "tags:work", opts)
		if err != nil {
			t.Fatalf("page2: %v", err)
		}
		if len(page2.Items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(page2.Items))
		}
		p2 := pathsFromItems(t, page2.Items)[0]
		if p1 == p2 {
			t.Fatalf("pagination repeated same item: %s", p1)
		}
	}
}

func TestDocFreqMaintenance_SQLite(t *testing.T) {
	schema := ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"tags": {Type: ministore.FieldKeyword, Multi: true},
		},
	}
	ix, _ := newIndex(t, schema)
	ctx := context.Background()

	put := func(path string, tags []string) {
		doc := map[string]any{"path": path, "tags": tags}
		b, _ := json.Marshal(doc)
		if err := ix.PutJSON(ctx, b); err != nil {
			t.Fatalf("PutJSON(%s): %v", path, err)
		}
	}

	put("/a", []string{"a", "b"})
	put("/b", []string{"b"})
	put("/a", []string{"c"}) // update: remove a,b; add c

	db := ix.DB()
	getFreq := func(val string) int64 {
		var df int64
		if err := db.QueryRowContext(ctx,
			"SELECT doc_freq FROM kw_dict WHERE field = ? AND value = ?",
			"tags", val,
		).Scan(&df); err != nil {
			t.Fatalf("scan doc_freq(%s): %v", val, err)
		}
		return df
	}

	if df := getFreq("a"); df != 0 {
		t.Fatalf("doc_freq(a)=%d want 0", df)
	}
	if df := getFreq("b"); df != 1 {
		t.Fatalf("doc_freq(b)=%d want 1", df)
	}
	if df := getFreq("c"); df != 1 {
		t.Fatalf("doc_freq(c)=%d want 1", df)
	}
}

func TestDiscoverValuesAndStats_SQLite(t *testing.T) {
	schema := ministore.Schema{
		Fields: map[string]ministore.FieldSpec{
			"tags":     {Type: ministore.FieldKeyword, Multi: true},
			"priority": {Type: ministore.FieldNumber},
		},
	}
	ix, _ := newIndex(t, schema)
	ctx := context.Background()

	put := func(path string, tags []string, pr int) {
		doc := map[string]any{"path": path, "tags": tags, "priority": pr}
		b, _ := json.Marshal(doc)
		if err := ix.PutJSON(ctx, b); err != nil {
			t.Fatalf("PutJSON(%s): %v", path, err)
		}
	}

	put("/1", []string{"x"}, 1)
	put("/2", []string{"x", "y"}, 2)
	put("/3", []string{"y"}, 3)
	put("/4", []string{"x"}, 4)

	// DiscoverValues tags
	vals, err := ix.DiscoverValues(ctx, "tags", "", 10)
	if err != nil {
		t.Fatalf("DiscoverValues: %v", err)
	}
	// Expect x:3, y:2 (order by freq desc)
	if len(vals) < 2 {
		t.Fatalf("expected at least 2 values, got %v", vals)
	}
	if vals[0].Value != "x" || vals[0].Count != 3 {
		t.Fatalf("want x:3 first, got %+v", vals[0])
	}
	if vals[1].Value != "y" || vals[1].Count != 2 {
		t.Fatalf("want y:2 second, got %+v", vals[1])
	}

	// Stats priority: values 1,2,3,4 => min=1 max=4 avg=2.5 median=2.5
	stats, err := ix.Stats(ctx, "priority", "")
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.Count != 4 {
		t.Fatalf("count=%d want 4", stats.Count)
	}
	if stats.Min == nil || *stats.Min != 1 {
		t.Fatalf("min=%v want 1", stats.Min)
	}
	if stats.Max == nil || *stats.Max != 4 {
		t.Fatalf("max=%v want 4", stats.Max)
	}
	if stats.Avg == nil || *stats.Avg != 2.5 {
		t.Fatalf("avg=%v want 2.5", stats.Avg)
	}
	if stats.Median == nil || *stats.Median != 2.5 {
		t.Fatalf("median=%v want 2.5", stats.Median)
	}

	// Filtered stats: priority>2 => values 3,4 => min=3 max=4 avg=3.5 median=3.5
	stats2, err := ix.Stats(ctx, "priority", "priority>2")
	if err != nil {
		t.Fatalf("Stats filtered: %v", err)
	}
	if stats2.Count != 2 {
		t.Fatalf("count=%d want 2", stats2.Count)
	}
	if stats2.Min == nil || *stats2.Min != 3 {
		t.Fatalf("min=%v want 3", stats2.Min)
	}
	if stats2.Max == nil || *stats2.Max != 4 {
		t.Fatalf("max=%v want 4", stats2.Max)
	}
	if stats2.Avg == nil || *stats2.Avg != 3.5 {
		t.Fatalf("avg=%v want 3.5", stats2.Avg)
	}
	if stats2.Median == nil || *stats2.Median != 3.5 {
		t.Fatalf("median=%v want 3.5", stats2.Median)
	}
}
