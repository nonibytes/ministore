package cli

import (
	"path/filepath"
	"strings"

	"github.com/nonibytes/ministore/internal/cliopt"
)

// ResolveIndexRef transforms the user-provided -i/--index value into a backend-specific reference.
//
//   - sqlite: if index contains a path separator or ends with .db, treat as explicit path.
//     else: <SQLitePath>/<name>.db
//   - postgres/redis: return the name as-is (namespacing handled by adapter).
func ResolveIndexRef(g cliopt.GlobalOptions, index string) string {
	switch strings.ToLower(g.Backend) {
	case "sqlite":
		if strings.Contains(index, string(filepath.Separator)) || strings.HasSuffix(index, ".db") {
			return index
		}
		return filepath.Join(g.SQLitePath, index+".db")
	default:
		return index
	}
}
