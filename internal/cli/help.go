package cli

import (
	"fmt"
	"io"
)

func PrintRootHelp(w io.Writer) {
	fmt.Fprintln(w, `ministore — single-file / pluggable search index (Go skeleton)

USAGE
  ministore [global flags] <command> [args]

GLOBAL FLAGS
  --backend sqlite|postgres|redis
  --sqlite-path <dir|file.db>
  --pg-dsn <dsn>
  --redis-addr <host:port>
  --redis-password <pw>
  --redis-db <n>

COMMANDS
  index <subcommand>
  put
  get
  peek
  delete
  search
  discover <subcommand>
  stats

Run "ministore <command> --help" for details (skeleton has minimal help today).`)
}
