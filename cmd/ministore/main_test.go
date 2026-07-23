package main

import "testing"

func TestIndexPathPrefersCommandLine(t *testing.T) {
	t.Setenv(indexEnvironmentVariable, "/env/index.db")
	a := parseArgs([]string{"--index", "/cli/index.db"})

	if got := a.indexPath(); got != "/cli/index.db" {
		t.Fatalf("indexPath() = %q, want command-line value", got)
	}
}

func TestIndexPathFallsBackToEnvironment(t *testing.T) {
	t.Setenv(indexEnvironmentVariable, "/env/index.db")
	a := parseArgs(nil)

	if got := a.indexPath(); got != "/env/index.db" {
		t.Fatalf("indexPath() = %q, want environment value", got)
	}
}

func TestIndexPathCanBeUnset(t *testing.T) {
	t.Setenv(indexEnvironmentVariable, "")
	a := parseArgs(nil)

	if got := a.indexPath(); got != "" {
		t.Fatalf("indexPath() = %q, want empty value", got)
	}
}
