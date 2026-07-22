package ministore_test

import (
	"context"
	"testing"

	"github.com/ministore/ministore/ministore"
)

func TestBatchPutJSONOwnsDecodedDocument(t *testing.T) {
	schema := ministore.Schema{Fields: map[string]ministore.FieldSpec{
		"title": {Type: ministore.FieldText},
	}}
	ix, _ := newIndex(t, schema)

	doc := []byte(`{"path":"/original","title":"Original"}`)
	batch := ministore.NewBatch()
	if err := batch.PutJSON(doc); err != nil {
		t.Fatalf("PutJSON: %v", err)
	}
	copy(doc, []byte(`{"path":"/mutated"`))

	count, err := batch.Execute(context.Background(), ix)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if count != 1 {
		t.Fatalf("Execute count = %d, want 1", count)
	}

	item, err := ix.Get(context.Background(), "/original")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(item.DocJSON) != `{"path":"/original","title":"Original"}` {
		t.Fatalf("stored document = %s", item.DocJSON)
	}
}

func TestBatchPutJSONRejectsInvalidPath(t *testing.T) {
	batch := ministore.NewBatch()
	for _, doc := range [][]byte{
		[]byte(`{"title":"missing"}`),
		[]byte(`{"path":""}`),
		[]byte(`{"path":42}`),
	} {
		if err := batch.PutJSON(doc); err == nil {
			t.Fatalf("PutJSON(%s) succeeded, want error", doc)
		}
	}
}
