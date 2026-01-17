package ministore

import "context"

// Background is provided so the CLI skeleton doesn't import context directly everywhere.
func Background() context.Context { return context.Background() }
