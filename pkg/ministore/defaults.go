package ministore

import "github.com/nonibytes/ministore/pkg/ministore/index"

func DefaultIndexOptions() index.IndexOptions {
	return index.IndexOptions{CursorTTL: DefaultCursorTTL}
}
