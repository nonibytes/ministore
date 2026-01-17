package util

import "time"

func NowMs(t time.Time) int64 {
	if t.IsZero() {
		t = time.Now()
	}
	return t.UnixNano() / int64(time.Millisecond)
}
