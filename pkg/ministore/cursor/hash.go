package cursor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/nonibytes/ministore/pkg/ministore/types"
)

func HashQuery(schemaJSON []byte, query string, rank types.RankMode) (string, error) {
	rb, err := json.Marshal(rank)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write(schemaJSON)
	h.Write([]byte("\n"))
	h.Write([]byte(query))
	h.Write([]byte("\n"))
	h.Write(rb)
	return hex.EncodeToString(h.Sum(nil)), nil
}
