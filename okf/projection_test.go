package okf

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ministore/ministore/ministore"
	"github.com/ministore/ministore/ministore/storage/sqlite"
)

func TestWalkProjectionsBuildsGraphAndCanonicalFields(t *testing.T) {
	root := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.md", "---\ntype: Note\ntags: [z, a, a]\nverified:\n  by: human:alice\n  at: 2026-01-02T03:04:05Z\n---\nHello [B](b.md).\n")
	write("b.md", "---\ntype: Note\n---\nWorld\n")
	var projections []Projection
	summary, err := WalkProjections(context.Background(), root, ValidateOptions{}, func(p Projection) error { projections = append(projections, p); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if summary.Errors != 0 || len(projections) != 2 {
		t.Fatalf("summary=%+v projections=%d", summary, len(projections))
	}
	if got := projections[0]["link_targets"]; !reflectJSON(got, []string{"/b"}) {
		t.Fatalf("links=%#v", got)
	}
	if got := projections[1]["backlinks"]; !reflectJSON(got, []string{"/a"}) {
		t.Fatalf("backlinks=%#v", got)
	}
	if projections[0]["trust_tier"] != "human-reviewed" || projections[0]["okf_projection_hash"] == "" {
		t.Fatalf("projection=%#v", projections[0])
	}
}

func TestSyncIsAtomicAndIdempotent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "one.md"), []byte("---\ntype: Note\n---\nBody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ix, err := ministore.Create(ctx, sqlite.New(filepath.Join(t.TempDir(), "index.db")), ProjectionSchema(), ministore.DefaultIndexOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	first, err := Sync(ctx, root, ix, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if first.Added != 1 || !first.OK {
		t.Fatalf("first=%+v", first)
	}
	second, err := Sync(ctx, root, ix, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Unchanged != 1 || second.Updated != 0 {
		t.Fatalf("second=%+v", second)
	}
	if _, err := ix.Get(ctx, "/one"); err != nil {
		t.Fatal(err)
	}
}

func reflectJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestMinimalProjectionGolden(t *testing.T) {
	var got []byte
	_, err := WalkProjections(context.Background(), filepath.Join("..", "testdata", "okf", "v0.2", "valid", "minimal"), ValidateOptions{}, func(p Projection) error { encoded, e := json.Marshal(p); got = append(encoded, '\n'); return e })
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("..", "testdata", "okf", "v0.2", "expected", "minimal-projection.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("projection golden mismatch\ngot %s\nwant %s", got, want)
	}
}

func TestSyncDryRunValidationAndDeleteLeaveTargetConsistent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	concept := filepath.Join(root, "one.md")
	if err := os.WriteFile(concept, []byte("---\ntype: Note\n---\nBody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ix, err := ministore.Create(ctx, sqlite.New(filepath.Join(t.TempDir(), "index.db")), ProjectionSchema(), ministore.DefaultIndexOptions())
	if err != nil {
		t.Fatal(err)
	}
	defer ix.Close()
	if _, err = Sync(ctx, root, ix, SyncOptions{}); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "two.md"), []byte("---\ntype: Note\n---\nTwo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err := Sync(ctx, root, ix, SyncOptions{DryRun: true})
	if err != nil || report.Added != 1 {
		t.Fatalf("dry run=%+v err=%v", report, err)
	}
	if _, err = ix.Get(ctx, "/two"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("dry run wrote /two: %v", err)
	}
	if err = os.WriteFile(concept, []byte("not frontmatter\n"), 0644); err != nil {
		t.Fatal(err)
	}
	report, err = Sync(ctx, root, ix, SyncOptions{})
	if err != nil || report.OK {
		t.Fatalf("invalid sync=%+v err=%v", report, err)
	}
	if _, err = ix.Get(ctx, "/one"); err != nil {
		t.Fatalf("invalid sync changed target: %v", err)
	}
	if err = os.WriteFile(concept, []byte("---\ntype: Note\n---\nBody\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(root, "two.md")); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(concept); err != nil {
		t.Fatal(err)
	}
	report, err = Sync(ctx, root, ix, SyncOptions{})
	if err != nil || report.Deleted != 1 {
		t.Fatalf("delete=%+v err=%v", report, err)
	}
	if _, err = ix.Get(ctx, "/one"); !ministore.IsKind(err, ministore.ErrNotFound) {
		t.Fatalf("delete retained /one: %v", err)
	}
}

func TestProjectionHashCanonicalizesSmallExponents(t *testing.T) {
	root := t.TempDir()
	raw := "---\ntype: Note\nsources:\n - resource: x\n   usage_count: 0.0000001\n---\nx\n"
	if err := os.WriteFile(filepath.Join(root, "x.md"), []byte(raw), 0644); err != nil {
		t.Fatal(err)
	}
	var hash string
	_, err := WalkProjections(context.Background(), root, ValidateOptions{}, func(p Projection) error { hash = p["okf_projection_hash"].(string); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if hash != "c4da14b0d649d845d0fdaac4ff318506ba5bf52841a2f7556d3f27cf2f68a5d3" {
		t.Fatalf("hash=%s", hash)
	}
}
