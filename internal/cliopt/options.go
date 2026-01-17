package cliopt

import "flag"

// GlobalOptions are parsed once at the CLI root and passed to subcommands.
// They intentionally mirror ministore.OpenOptions plus output flags.
//
// NOTE: This is a separate package to avoid import cycles between the root
// command router and per-command code.
type GlobalOptions struct {
	Backend       string
	SQLitePath    string
	PostgresDSN   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	Format  string
	Explain bool
}

func DefaultGlobalOptions() GlobalOptions {
	return GlobalOptions{
		Backend:    "sqlite",
		SQLitePath: ".",
	}
}

func BindGlobalFlags(fs *flag.FlagSet, g *GlobalOptions) {
	fs.StringVar(&g.Backend, "backend", g.Backend, "backend: sqlite|postgres|redis")

	fs.StringVar(&g.SQLitePath, "sqlite-path", g.SQLitePath, "sqlite directory or explicit .db file path")

	fs.StringVar(&g.PostgresDSN, "pg-dsn", g.PostgresDSN, "postgres DSN")

	fs.StringVar(&g.RedisAddr, "redis-addr", g.RedisAddr, "redis address host:port")
	fs.StringVar(&g.RedisPassword, "redis-password", g.RedisPassword, "redis password")
	fs.IntVar(&g.RedisDB, "redis-db", g.RedisDB, "redis db number")
}
