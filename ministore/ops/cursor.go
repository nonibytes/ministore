package ops

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ministore/ministore/ministore/storage"
)

const shortCursorPrefix = "c:"

// DBCursorStore implements CursorStore backed by database
type DBCursorStore struct {
	db   *sql.DB
	sqlt storage.SQL
	ttl  time.Duration
}

// NewDBCursorStore creates a new database-backed cursor store
func NewDBCursorStore(db *sql.DB, sqlt storage.SQL, ttl time.Duration) *DBCursorStore {
	return &DBCursorStore{
		db:   db,
		sqlt: sqlt,
		ttl:  ttl,
	}
}

// Resolve resolves a cursor token to its payload
func (s *DBCursorStore) Resolve(ctx context.Context, token string) (*CursorPayload, error) {
	if strings.HasPrefix(token, shortCursorPrefix) {
		// Short cursor - load from database
		handle := token[len(shortCursorPrefix):]
		return s.resolveShort(ctx, handle)
	}

	// Full cursor - decode from base64
	return s.resolveFull(token)
}

// Store stores a cursor payload and returns a token
func (s *DBCursorStore) Store(ctx context.Context, payload CursorPayload, mode CursorMode) (string, error) {
	if mode == CursorShort {
		return s.storeShort(ctx, payload)
	}
	return s.storeFull(payload)
}

// CleanupExpired removes expired cursors
func (s *DBCursorStore) CleanupExpired(ctx context.Context) error {
	nowMS := time.Now().UnixMilli()
	_, err := s.db.ExecContext(ctx, s.sqlt.CleanupExpiredCursors, nowMS)
	return err
}

func (s *DBCursorStore) resolveShort(ctx context.Context, handle string) (*CursorPayload, error) {
	var payloadJSON string
	var expiresAt int64
	err := s.db.QueryRowContext(ctx, s.sqlt.GetCursor, handle).Scan(&payloadJSON, &expiresAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("cursor expired or not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query cursor: %w", err)
	}

	// Check expiration
	if time.Now().UnixMilli() > expiresAt {
		return nil, fmt.Errorf("cursor expired")
	}

	var payload CursorPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("unmarshal cursor payload: %w", err)
	}

	return &payload, nil
}

func (s *DBCursorStore) resolveFull(token string) (*CursorPayload, error) {
	// Decode base64url (no padding)
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}

	var payload CursorPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}

	return &payload, nil
}

func (s *DBCursorStore) storeShort(ctx context.Context, payload CursorPayload) (string, error) {
	// Generate handle
	handle, err := makeShortHandle()
	if err != nil {
		return "", fmt.Errorf("generate handle: %w", err)
	}

	// Serialize payload
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	nowMS := time.Now().UnixMilli()
	expiresAtMS := nowMS + s.ttl.Milliseconds()

	_, err = s.db.ExecContext(ctx, s.sqlt.PutCursor, handle, string(payloadJSON), nowMS, expiresAtMS)
	if err != nil {
		return "", fmt.Errorf("store cursor: %w", err)
	}

	return shortCursorPrefix + handle, nil
}

func (s *DBCursorStore) storeFull(payload CursorPayload) (string, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	// Encode as base64url without padding
	return base64.RawURLEncoding.EncodeToString(payloadJSON), nil
}

func makeShortHandle() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IsShortCursorToken returns true if the token is a short cursor
func IsShortCursorToken(token string) bool {
	return strings.HasPrefix(token, shortCursorPrefix)
}

// SimpleCursorStore is a simple in-memory implementation for testing
type SimpleCursorStore struct{}

func (s *SimpleCursorStore) Resolve(ctx context.Context, token string) (*CursorPayload, error) {
	// For simple store, always use full cursors
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}

	var payload CursorPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}

	return &payload, nil
}

func (s *SimpleCursorStore) Store(ctx context.Context, payload CursorPayload, mode CursorMode) (string, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payloadJSON), nil
}
