package okf

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const defaultTargetVersion = "0.2"

// ValidateBundle enumerates a bundle without following symlinks, validates
// concept base conformance one document at a time, and emits findings from a
// private disk-backed stage in deterministic order.
func ValidateBundle(ctx context.Context, root string, opts ValidateOptions, emit FindingSink) (summary ValidationSummary, err error) {
	stage, summary, err := prepareBundle(ctx, root, opts)
	if err != nil {
		return ValidationSummary{}, err
	}
	defer func() {
		if closeErr := stage.close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	if err := stage.emitFindings(ctx, emit); err != nil {
		return ValidationSummary{}, fmt.Errorf("emit OKF validation finding: %w", err)
	}
	return summary, nil
}

func prepareBundle(ctx context.Context, root string, opts ValidateOptions) (stage *validationStage, summary ValidationSummary, err error) {
	targetVersion := opts.TargetVersion
	if targetVersion == "" {
		targetVersion = defaultTargetVersion
	}

	info, err := os.Lstat(root)
	if err != nil {
		return nil, ValidationSummary{}, fmt.Errorf("inspect OKF bundle root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ValidationSummary{}, fmt.Errorf("OKF bundle root is not a directory: %s", root)
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, ValidationSummary{}, fmt.Errorf("resolve OKF bundle root: %w", err)
	}

	stage, err = newValidationStage("")
	if err != nil {
		return nil, ValidationSummary{}, err
	}
	cleanupStage := stage
	failed := true
	defer func() {
		if failed {
			_ = cleanupStage.close()
		}
	}()

	tx, err := stage.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, ValidationSummary{}, fmt.Errorf("begin OKF staging transaction: %w", err)
	}
	if err := enumerateBundle(ctx, tx, absoluteRoot); err != nil {
		_ = tx.Rollback()
		return nil, ValidationSummary{}, err
	}
	if err := tx.Commit(); err != nil {
		return nil, ValidationSummary{}, fmt.Errorf("commit OKF entry staging: %w", err)
	}

	if err := validateStagedConcepts(ctx, stage, absoluteRoot); err != nil {
		return nil, ValidationSummary{}, err
	}
	if err := resolveStagedLinks(ctx, stage); err != nil {
		return nil, ValidationSummary{}, err
	}
	declaredVersion, err := validateStagedReservedFiles(ctx, stage, absoluteRoot)
	if err != nil {
		return nil, ValidationSummary{}, err
	}
	summary, err = stage.summary(ctx, absoluteRoot, targetVersion)
	if err != nil {
		return nil, ValidationSummary{}, fmt.Errorf("summarize OKF validation: %w", err)
	}
	summary.DeclaredVersion = declaredVersion
	failed = false
	return stage, summary, nil
}

func enumerateBundle(ctx context.Context, tx *sql.Tx, root string) error {
	return enumerateDirectory(ctx, tx, root, root)
}

func enumerateDirectory(ctx context.Context, tx *sql.Tx, root, directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return fmt.Errorf("open OKF bundle directory: %w", err)
	}
	defer handle.Close()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		entries, err := handle.ReadDir(1)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read OKF bundle directory: %w", err)
		}
		entry := entries[0]
		path := filepath.Join(directory, entry.Name())
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make OKF path relative: %w", err)
		}
		relative = filepath.ToSlash(relative)
		if !utf8.ValidString(relative) {
			return fmt.Errorf("OKF bundle path is not valid UTF-8")
		}

		kind := classifyEntry(entry)
		if err := insertEntry(ctx, tx, relative, kind); err != nil {
			return fmt.Errorf("stage OKF entry %q: %w", relative, err)
		}
		if kind == "ignored" && !entry.IsDir() {
			finding := Finding{
				Severity: SeverityWarning, Code: CodeIgnoredSpecialFile,
				Path: relative, SpecSection: "4.1",
				Message: "symlink or special file is ignored",
			}
			if err := insertFinding(ctx, tx, finding); err != nil {
				return fmt.Errorf("stage OKF finding for %q: %w", relative, err)
			}
		}
		if entry.IsDir() {
			if err := enumerateDirectory(ctx, tx, root, path); err != nil {
				return err
			}
		}
	}
}

func classifyEntry(entry os.DirEntry) string {
	if entry.IsDir() {
		return "ignored"
	}
	if entry.Type().IsRegular() {
		switch entry.Name() {
		case "index.md":
			return "index"
		case "log.md":
			return "log"
		default:
			if strings.HasSuffix(entry.Name(), ".md") {
				return "concept"
			}
			return "ancillary"
		}
	}
	return "ignored"
}

func validateStagedConcepts(ctx context.Context, stage *validationStage, root string) error {
	after := ""
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, ok, err := stage.nextEntryPath(ctx, "concept", after)
		if err != nil {
			return fmt.Errorf("read staged OKF concept path: %w", err)
		}
		if !ok {
			return nil
		}
		after = relative

		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			return fmt.Errorf("read OKF concept %q: %w", relative, err)
		}
		document, findings, err := ParseDocument(relative, raw)
		if err != nil {
			return fmt.Errorf("parse OKF concept %q: %w", relative, err)
		}
		findings = append(findings, validateConceptBase(document)...)
		findings = append(findings, validateConceptAdvisories(document)...)
		resourceFindings, err := validateAttestedResources(ctx, stage, document)
		if err != nil {
			return fmt.Errorf("validate OKF computation resources for %q: %w", relative, err)
		}
		findings = append(findings, resourceFindings...)

		tx, err := stage.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin OKF finding transaction: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO concepts(path, raw) VALUES (?, ?)`, relative, raw); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("stage OKF concept %q: %w", relative, err)
		}
		for _, candidate := range extractLinkCandidates(document) {
			if _, err := tx.ExecContext(ctx, `INSERT INTO link_candidates(source,destination,line,column) VALUES (?,?,?,?)`, relative, candidate.destination, candidate.line, candidate.column); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("stage OKF link from %q: %w", relative, err)
			}
		}
		for _, finding := range findings {
			if err := insertFinding(ctx, tx, finding); err != nil {
				_ = tx.Rollback()
				return fmt.Errorf("stage OKF finding for %q: %w", relative, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit OKF findings for %q: %w", relative, err)
		}
	}
}

func validateConceptBase(document Document) []Finding {
	metadata := document.Metadata()
	if metadata == nil {
		return nil
	}
	typeNode, exists := metadata.Lookup("type")
	value, valid := metadata.String("type")
	if valid && strings.TrimSpace(value) != "" {
		return nil
	}

	var line, column *int
	if exists {
		typeNode = ResolveAlias(typeNode)
		if typeNode != nil {
			line, column = position(typeNode.Line+1), position(typeNode.Column)
		}
	}
	return []Finding{{
		Severity: SeverityError, Code: CodeMissingType, Path: document.Path,
		Line: line, Column: column, SpecSection: "4.1",
		Message: "type must be a non-empty string",
	}}
}
