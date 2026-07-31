package okf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/ministore/ministore/ministore"
)

func Sync(ctx context.Context, root string, ix *ministore.Index, opts SyncOptions) (report SyncReport, err error) {
	started := time.Now()
	if ix == nil {
		return report, fmt.Errorf("OKF sync requires an open index")
	}
	stage, summary, err := prepareBundle(ctx, root, ValidateOptions{TargetVersion: opts.TargetVersion})
	if err != nil {
		return report, err
	}
	defer func() {
		report.DurationMS = time.Since(started).Milliseconds()
		if closeErr := stage.close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()
	report = SyncReport{Bundle: summary.Bundle, ProjectionVersion: ProjectionVersion, Concepts: summary.Concepts, Validation: summary}
	if summary.Errors > 0 || (opts.Strict && summary.Warnings > 0) {
		return report, nil
	}
	if !reflect.DeepEqual(ix.Schema(), ProjectionSchema()) {
		return report, fmt.Errorf("target index schema does not equal the OKF projection schema")
	}
	if err := ix.ScanPaths(ctx, "", func(p string) error {
		_, e := stage.db.ExecContext(ctx, `INSERT INTO existing_paths(path) VALUES (?)`, p)
		return e
	}); err != nil {
		return report, err
	}
	after := ""
	for {
		var source string
		var raw []byte
		e := stage.db.QueryRowContext(ctx, `SELECT path,raw FROM concepts WHERE path>? COLLATE BINARY ORDER BY path COLLATE BINARY LIMIT 1`, after).Scan(&source, &raw)
		if e == sql.ErrNoRows {
			break
		}
		if e != nil {
			return report, e
		}
		after = source
		projection, e := stage.project(ctx, source, raw, summary.TargetVersion)
		if e != nil {
			return report, e
		}
		target := projection["path"].(string)
		kind := "add"
		var exists int
		e = stage.db.QueryRowContext(ctx, `SELECT 1 FROM existing_paths WHERE path=?`, target).Scan(&exists)
		if e == nil {
			kind = "update"
			view, getErr := ix.Get(ctx, target)
			if getErr != nil {
				return report, getErr
			}
			var current map[string]any
			if json.Unmarshal(view.DocJSON, &current) == nil && current["okf_projection_hash"] == projection["okf_projection_hash"] {
				kind = "unchanged"
			}
		} else if e != sql.ErrNoRows {
			return report, e
		}
		if _, e = stage.db.ExecContext(ctx, `INSERT INTO actions(path,kind) VALUES (?,?)`, target, kind); e != nil {
			return report, e
		}
	}
	if _, err = stage.db.ExecContext(ctx, `INSERT INTO actions(path,kind) SELECT path,'delete' FROM existing_paths WHERE NOT EXISTS(SELECT 1 FROM actions WHERE actions.path=existing_paths.path)`); err != nil {
		return report, err
	}
	if err = stage.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(kind='add'),0),COALESCE(SUM(kind='update'),0),COALESCE(SUM(kind='unchanged'),0),COALESCE(SUM(kind='delete'),0) FROM actions`).Scan(&report.Added, &report.Updated, &report.Unchanged, &report.Deleted); err != nil {
		return report, err
	}
	report.OK = true
	if opts.DryRun {
		return report, nil
	}
	_, err = ix.WriteBatch(ctx, func(writer *ministore.BatchWriter) error {
		after := ""
		for {
			var target, kind string
			e := stage.db.QueryRowContext(ctx, `SELECT path,kind FROM actions WHERE kind!='unchanged' AND path>? COLLATE BINARY ORDER BY path COLLATE BINARY LIMIT 1`, after).Scan(&target, &kind)
			if e == sql.ErrNoRows {
				return nil
			}
			if e != nil {
				return e
			}
			after = target
			if kind == "delete" {
				if e := writer.Delete(target); e != nil {
					return e
				}
				continue
			}
			source := target[1:] + ".md"
			var raw []byte
			if e := stage.db.QueryRowContext(ctx, `SELECT raw FROM concepts WHERE path=?`, source).Scan(&raw); e != nil {
				return e
			}
			p, e := stage.project(ctx, source, raw, summary.TargetVersion)
			if e != nil {
				return e
			}
			encoded, e := json.Marshal(p)
			if e != nil {
				return e
			}
			if e := writer.PutJSON(encoded); e != nil {
				return e
			}
		}
	})
	if err != nil {
		return report, err
	}
	return report, nil
}
