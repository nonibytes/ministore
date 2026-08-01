package okf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"golang.org/x/text/cases"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const validationStageDDL = `
CREATE TABLE entries (
    path TEXT PRIMARY KEY COLLATE BINARY,
    kind TEXT NOT NULL,
    folded_path TEXT NOT NULL
);
CREATE TABLE concepts (
    path TEXT PRIMARY KEY COLLATE BINARY,
    raw BLOB NOT NULL
);
CREATE TABLE link_candidates (
    id INTEGER PRIMARY KEY,
    source TEXT NOT NULL COLLATE BINARY,
    destination TEXT NOT NULL,
    line INTEGER,
    column INTEGER
);
CREATE TABLE edges (
    source TEXT NOT NULL COLLATE BINARY,
    target TEXT NOT NULL COLLATE BINARY,
    PRIMARY KEY(source, target)
);
CREATE TABLE existing_paths (path TEXT PRIMARY KEY COLLATE BINARY);
CREATE TABLE actions (
    path TEXT PRIMARY KEY COLLATE BINARY,
    kind TEXT NOT NULL
);
CREATE TABLE findings (
    id INTEGER PRIMARY KEY,
    severity TEXT NOT NULL,
    code TEXT NOT NULL,
    path TEXT NOT NULL COLLATE BINARY,
    line INTEGER,
    column INTEGER,
    spec_section TEXT,
    finding_json BLOB NOT NULL
);
CREATE INDEX findings_order ON findings(
    path COLLATE BINARY,
    COALESCE(line, 0),
    COALESCE(column, 0),
    CASE severity WHEN 'error' THEN 0 ELSE 1 END,
    code,
    id
);`

type validationStage struct {
	db        *sql.DB
	directory string
	path      string
}

func newValidationStage(parent string) (*validationStage, error) {
	directory, err := os.MkdirTemp(parent, "ministore-okf-")
	if err != nil {
		return nil, fmt.Errorf("create OKF staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(directory)
		}
	}()

	path := filepath.Join(directory, "stage.sqlite3")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create OKF staging database: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close OKF staging database: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open OKF staging database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(validationStageDDL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize OKF staging database: %w", err)
	}

	cleanup = false
	return &validationStage{db: db, directory: directory, path: path}, nil
}

func (s *validationStage) close() error {
	var closeErr error
	if s.db != nil {
		closeErr = s.db.Close()
		s.db = nil
	}
	removeErr := os.RemoveAll(s.directory)
	if closeErr != nil {
		return fmt.Errorf("close OKF staging database: %w", closeErr)
	}
	if removeErr != nil {
		return fmt.Errorf("remove OKF staging directory: %w", removeErr)
	}
	return nil
}

func insertEntry(ctx context.Context, tx *sql.Tx, path, kind string) error {
	folded := cases.Fold().String(path)
	var existing string
	err := tx.QueryRowContext(ctx, `SELECT path FROM entries WHERE folded_path=? AND path<>? LIMIT 1`, folded, path).Scan(&existing)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO entries(path, kind, folded_path) VALUES (?, ?, ?)`, path, kind, folded); err != nil {
		return err
	}
	if existing != "" {
		return insertFinding(ctx, tx, Finding{Severity: SeverityWarning, Code: CodeCaseFoldCollision, Path: path, SpecSection: "4.1", Message: "path collides under Unicode case folding with " + existing})
	}
	return nil
}

func insertFinding(ctx context.Context, tx *sql.Tx, finding Finding) error {
	encoded, err := json.Marshal(finding)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO findings(severity, code, path, line, column, spec_section, finding_json)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		finding.Severity, finding.Code, finding.Path,
		nullablePosition(finding.Line), nullablePosition(finding.Column),
		nullableString(finding.SpecSection), encoded,
	)
	return err
}

func nullablePosition(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func (s *validationStage) nextEntryPath(ctx context.Context, kind, after string) (string, bool, error) {
	var path string
	err := s.db.QueryRowContext(ctx, `
SELECT path FROM entries
WHERE kind = ? AND path > ? COLLATE BINARY
ORDER BY path COLLATE BINARY
LIMIT 1`, kind, after).Scan(&path)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return path, true, nil
}

func (s *validationStage) summary(ctx context.Context, bundle, targetVersion string) (ValidationSummary, error) {
	summary := ValidationSummary{TargetVersion: targetVersion, Bundle: bundle}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM entries WHERE kind = 'concept'`).Scan(&summary.Concepts); err != nil {
		return ValidationSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT
    COALESCE(SUM(CASE WHEN severity = 'error' THEN 1 ELSE 0 END), 0),
    COALESCE(SUM(CASE WHEN severity = 'warning' THEN 1 ELSE 0 END), 0)
FROM findings`).Scan(&summary.Errors, &summary.Warnings); err != nil {
		return ValidationSummary{}, err
	}
	return summary, nil
}

func (s *validationStage) emitFindings(ctx context.Context, emit FindingSink) error {
	if emit == nil {
		return nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT finding_json FROM findings
ORDER BY
    path COLLATE BINARY,
    COALESCE(line, 0),
    COALESCE(column, 0),
    CASE severity WHEN 'error' THEN 0 ELSE 1 END,
    code,
    id`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return err
		}
		var finding Finding
		if err := json.Unmarshal(encoded, &finding); err != nil {
			return err
		}
		if err := emit(finding); err != nil {
			return err
		}
	}
	return rows.Err()
}
