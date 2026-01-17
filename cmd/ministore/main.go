package main

import (
	"os"

	"github.com/nonibytes/ministore/internal/cli"
)

func main() {
	code := cli.Execute(os.Args[1:])
	os.Exit(code)
}
