package cursor

import (
	"encoding/base64"
	"encoding/json"

	mserrors "github.com/nonibytes/ministore/pkg/ministore/errors"
)

func EncodeFull(pos Position) (string, error) {
	b, err := json.Marshal(pos)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func DecodeFull(tok string) (Position, error) {
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		return Position{}, mserrors.Wrap(mserrors.ErrCursor, "base64 decode", err)
	}
	var pos Position
	if err := json.Unmarshal(b, &pos); err != nil {
		return Position{}, mserrors.Wrap(mserrors.ErrCursor, "json parse", err)
	}
	return pos, nil
}

func IsShortToken(tok string) bool {
	return len(tok) > 2 && tok[0:2] == "c:"
}

func ShortHandle(tok string) (string, bool) {
	if !IsShortToken(tok) {
		return "", false
	}
	return tok[2:], true
}
