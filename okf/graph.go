package okf

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

type linkCandidate struct {
	destination  string
	line, column int
}

func validateAttestedResources(ctx context.Context, stage *validationStage, document Document) ([]Finding, error) {
	m := document.Metadata()
	if m == nil || metadataString(m, "type") != "Attested Computation" {
		return nil, nil
	}
	type ref struct {
		value string
		node  *yaml.Node
		code  FindingCode
		label string
	}
	var refs []ref
	if n, ok := m.Lookup("computation"); ok {
		if v, valid := NodeString(n); valid && v != "" {
			refs = append(refs, ref{v, n, CodeMalformedComputation, "computation"})
		}
	}
	for key, code := range map[string]FindingCode{"executor": CodeMalformedExecutor, "attester": CodeMalformedAttester} {
		if n, ok := m.Lookup(key); ok {
			if resource, ok := MappingLookup(n, "resource"); ok {
				if v, valid := NodeString(resource); valid && v != "" {
					refs = append(refs, ref{v, resource, code, key + " resource"})
				}
			}
		}
	}
	var findings []Finding
	for _, r := range refs {
		target, external, code := normalizeLink(document.Path, r.value)
		if external {
			continue
		}
		missing := code != ""
		if !missing {
			var exists int
			err := stage.db.QueryRowContext(ctx, `SELECT 1 FROM entries WHERE path=?`, target).Scan(&exists)
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
			missing = err == sql.ErrNoRows
		}
		if missing {
			findings = append(findings, advisoryFinding(r.code, document.Path, r.node, r.label+" local path does not exist", "10.3"))
		}
	}
	return findings, nil
}

func extractLinkCandidates(document Document) []linkCandidate {
	if document.Body() == nil {
		return nil
	}
	root := goldmark.DefaultParser().Parse(text.NewReader(document.Body()))
	var out []linkCandidate
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		destination := string(link.Destination)
		if destination == "" || strings.HasPrefix(destination, "#") {
			return ast.WalkContinue, nil
		}
		offset := document.body.start
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			if source, ok := child.(*ast.Text); ok {
				offset += source.Segment.Start
				break
			}
		}
		line, column := bytePosition(document.Raw(), offset)
		out = append(out, linkCandidate{destination, line, column})
		return ast.WalkContinue, nil
	})
	return out
}

func resolveStagedLinks(ctx context.Context, stage *validationStage) error {
	var after int64
	for {
		var candidate struct {
			id                  int64
			source, destination string
			line, column        int
		}
		err := stage.db.QueryRowContext(ctx, `SELECT id,source,destination,line,column FROM link_candidates WHERE id>? ORDER BY id LIMIT 1`, after).Scan(&candidate.id, &candidate.source, &candidate.destination, &candidate.line, &candidate.column)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read staged OKF links: %w", err)
		}
		after = candidate.id
		target, external, code := normalizeLink(candidate.source, candidate.destination)
		if external {
			continue
		}
		if code != "" {
			if err := stageLinkFinding(ctx, stage, candidate.source, candidate.line, candidate.column, code, candidate.destination); err != nil {
				return err
			}
			continue
		}
		var kind string
		err = stage.db.QueryRowContext(ctx, `SELECT kind FROM entries WHERE path=?`, target).Scan(&kind)
		if err == sql.ErrNoRows {
			if err := stageLinkFinding(ctx, stage, candidate.source, candidate.line, candidate.column, CodeMissingLinkTarget, candidate.destination); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if kind == "concept" {
			if _, err := stage.db.ExecContext(ctx, `INSERT OR IGNORE INTO edges(source,target) VALUES (?,?)`, candidate.source, target); err != nil {
				return err
			}
		}
	}
}

func normalizeLink(source, destination string) (string, bool, FindingCode) {
	u, err := url.Parse(destination)
	if err != nil {
		return "", false, CodeUnsafePercentEncoding
	}
	if u.Scheme != "" || u.Host != "" || strings.HasPrefix(destination, "//") {
		return "", true, ""
	}
	rawPath := u.EscapedPath()
	if rawPath == "" {
		return "", true, ""
	}
	for _, segment := range strings.Split(rawPath, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil || strings.ContainsAny(decoded, "/\\\x00") {
			return "", false, CodeUnsafePercentEncoding
		}
	}
	decoded, err := url.PathUnescape(rawPath)
	if err != nil {
		return "", false, CodeUnsafePercentEncoding
	}
	var joined string
	if strings.HasPrefix(decoded, "/") {
		joined = strings.TrimPrefix(decoded, "/")
	} else {
		joined = path.Join(path.Dir(source), decoded)
	}
	clean := path.Clean(joined)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", false, CodeLinkEscapesRoot
	}
	return clean, false, ""
}

func stageLinkFinding(ctx context.Context, stage *validationStage, source string, line, column int, code FindingCode, destination string) error {
	message := "local link target does not exist"
	if code == CodeLinkEscapesRoot {
		message = "local link escapes the bundle root"
	}
	if code == CodeUnsafePercentEncoding {
		message = "local link contains unsafe percent encoding"
	}
	tx, err := stage.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	err = insertFinding(ctx, tx, Finding{Severity: SeverityWarning, Code: code, Path: source, Line: position(line), Column: position(column), SpecSection: "4.1", Message: message + ": " + destination})
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
