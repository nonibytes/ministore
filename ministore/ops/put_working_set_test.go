package ops

import (
	"context"
	"testing"
)

func TestPutExecutorFlushWorkingSetClearsCaches(t *testing.T) {
	w := &PutExecutor{
		keywordIDs:   map[keywordCacheKey]int64{{field: "tag", value: "a"}: 1},
		docFreqDelta: map[int64]int64{1: 0},
	}
	if err := w.FlushWorkingSet(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(w.keywordIDs) != 0 || len(w.docFreqDelta) != 0 {
		t.Fatalf("working set not cleared: keywords=%d docfreq=%d", len(w.keywordIDs), len(w.docFreqDelta))
	}
}
