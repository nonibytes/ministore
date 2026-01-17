package ministore

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

type CursorPayloadKind string

const (
	CurFts     CursorPayloadKind = "fts"
	CurRecency CursorPayloadKind = "recency"
	CurField   CursorPayloadKind = "field"
	CurNone    CursorPayloadKind = "none"
)

type CursorPayload struct {
	Kind CursorPayloadKind `json:"kind"`

	// fts
	Score  float64 `json:"score,omitempty"`
	ItemID int64   `json:"item_id,omitempty"`

	// recency / field
	UpdatedAtMS int64  `json:"updated_at_ms,omitempty"`
	Path        string `json:"path,omitempty"`

	// field
	Field     string  `json:"field,omitempty"`
	RankValue float64 `json:"rank_value,omitempty"`
}

type CursorPosition struct {
	Payload CursorPayload `json:"payload"`
	Hash    string        `json:"hash"`
}

type rankModeSer struct {
	Kind  RankModeKind `json:"kind"`
	Field string       `json:"field,omitempty"`
}

func hashQuery(schemaJSON []byte, query string, rank RankMode) (string, error) {
	r := rankModeSer{Kind: rank.Kind}
	if rank.Kind == RankField {
		r.Field = rank.Field
	}
	rb, err := json.Marshal(r)
	if err != nil {
		return "", Wrap(ErrCursor, "rank json", err)
	}
	h := sha256.New()
	h.Write(schemaJSON)
	h.Write([]byte("\n"))
	h.Write([]byte(query))
	h.Write([]byte("\n"))
	h.Write(rb)
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func encodeFull(pos CursorPosition) (string, error) {
	b, err := json.Marshal(pos)
	if err != nil {
		return "", Wrap(ErrCursor, "cursor json", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeFull(tok string) (CursorPosition, error) {
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return CursorPosition{}, New(ErrCursor, "base64 decode error")
	}
	var pos CursorPosition
	if err := json.Unmarshal(b, &pos); err != nil {
		return CursorPosition{}, New(ErrCursor, "cursor json parse error")
	}
	return pos, nil
}

func isShortCursor(tok string) bool {
	return len(tok) > 2 && tok[:2] == "c:"
}

func shortHandle(tok string) (string, bool) {
	if !isShortCursor(tok) {
		return "", false
	}
	return tok[2:], true
}

func makeShortHandle() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", Wrap(ErrCursor, "rand", err)
	}
	return hex.EncodeToString(b), nil
}
