package ministore

import (
	"context"

	"github.com/nonibytes/ministore/internal/cliopt"
	"github.com/nonibytes/ministore/pkg/ministore/adapters/postgres"
	"github.com/nonibytes/ministore/pkg/ministore/adapters/redis"
	"github.com/nonibytes/ministore/pkg/ministore/adapters/sqlite"
	"github.com/nonibytes/ministore/pkg/ministore/backend"
)

type OpenOptions struct {
	Backend       string
	SQLitePath    string
	PostgresDSN   string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
}

// OpenOptionsFromCLI converts CLI global flags into library open options.
func OpenOptionsFromCLI(g cliopt.GlobalOptions) OpenOptions {
	return OpenOptions{
		Backend:       g.Backend,
		SQLitePath:    g.SQLitePath,
		PostgresDSN:   g.PostgresDSN,
		RedisAddr:     g.RedisAddr,
		RedisPassword: g.RedisPassword,
		RedisDB:       g.RedisDB,
	}
}

// Open selects a backend implementation.
func Open(ctx context.Context, opts OpenOptions) (*Client, error) {
	var b backend.Backend
	switch opts.Backend {
	case "sqlite":
		b = sqlite.NewBackend(sqlite.Options{Path: opts.SQLitePath})
	case "postgres":
		b = postgres.NewBackend(postgres.Options{DSN: opts.PostgresDSN})
	case "redis":
		b = redis.NewBackend(redis.Options{Addr: opts.RedisAddr, Password: opts.RedisPassword, DB: opts.RedisDB})
	default:
		return nil, NewError(ErrBackend, "unknown backend")
	}
	c := NewClient(b)
	_ = ctx // backend might want to verify connectivity in future
	return c, nil
}
