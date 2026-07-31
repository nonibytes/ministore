package okf

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func validateStagedReservedFiles(ctx context.Context, stage *validationStage, root string) error {
	for _, kind := range []string{"index", "log"} {
		after := ""
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			relative, ok, err := stage.nextEntryPath(ctx, kind, after)
			if err != nil {
				return fmt.Errorf("read staged OKF %s path: %w", kind, err)
			}
			if !ok {
				break
			}
			after = relative
			raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if err != nil {
				return fmt.Errorf("read OKF %s %q: %w", kind, relative, err)
			}
			var findings []Finding
			if kind == "index" {
				findings = validateIndex(relative, raw)
			} else {
				findings = validateLog(relative, raw)
			}
			tx, err := stage.db.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("begin OKF reserved finding transaction: %w", err)
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
	return nil
}

func validateIndex(path string, raw []byte) []Finding {
	body, bodyStart, prefixFindings := indexBody(path, raw)
	if body == nil {
		return prefixFindings
	}
	document := goldmark.DefaultParser().Parse(text.NewReader(body))
	findings := prefixFindings
	seenHeading, sectionHasList := false, false
	var structuralOffset *int
	markMalformed := func(node ast.Node) {
		if structuralOffset != nil {
			return
		}
		offset := blockOffset(node)
		structuralOffset = &offset
	}
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		switch current := node.(type) {
		case *ast.Heading:
			if seenHeading && !sectionHasList {
				markMalformed(node)
			}
			seenHeading, sectionHasList = true, false
		case *ast.List:
			if !seenHeading {
				markMalformed(node)
			} else {
				sectionHasList = true
			}
			for item := current.FirstChild(); item != nil; item = item.NextSibling() {
				if !listItemIsLinkFirst(item, body) {
					offset := lineStartOffset(body, blockOffset(item))
					line, column := bytePosition(raw, bodyStart+offset)
					findings = append(findings, reservedFinding(CodeIndexEntryNotLinkFirst, path, line, column, "index list entry must begin with a Markdown link"))
				}
			}
		case *ast.LinkReferenceDefinition:
			// Reference definitions are declarations for links in list items,
			// not prose content in an index section.
		default:
			markMalformed(node)
		}
	}
	if !seenHeading || !sectionHasList {
		if structuralOffset == nil {
			offset := len(body)
			structuralOffset = &offset
		}
	}
	if structuralOffset != nil {
		line, column := bytePosition(raw, bodyStart+*structuralOffset)
		findings = append(findings, reservedFinding(CodeMalformedIndex, path, line, column, "index must contain heading sections made of link-first lists"))
	}
	return findings
}

func indexBody(path string, raw []byte) ([]byte, int, []Finding) {
	if !utf8.Valid(raw) {
		line, column := invalidUTF8Position(raw)
		return nil, 0, []Finding{newParseFinding(SeverityError, CodeInvalidUTF8, path, line, column, "index is not valid UTF-8")}
	}
	contentStart := 0
	if bytes.HasPrefix(raw, utf8BOM) {
		contentStart = len(utf8BOM)
	}
	contentEnd, _ := physicalLine(raw, contentStart)
	if strings.TrimSpace(string(raw[contentStart:contentEnd])) != "---" {
		return raw, 0, nil
	}
	document, parsed, _ := ParseDocument(path, raw)
	if path != "index.md" {
		finding := reservedFinding(CodeNestedIndexFrontmatter, path, 1, 1, "only the root index may have frontmatter")
		if document.body.valid {
			return document.Body(), document.body.start, []Finding{finding}
		}
		return nil, 0, []Finding{finding}
	}
	if !document.body.valid {
		return nil, 0, parsed
	}
	return document.Body(), document.body.start, parsed
}

func listItemIsLinkFirst(item ast.Node, source []byte) bool {
	block := item.FirstChild()
	if block == nil {
		return false
	}
	for inline := block.FirstChild(); inline != nil; inline = inline.NextSibling() {
		if textNode, ok := inline.(*ast.Text); ok && strings.TrimSpace(string(textNode.Segment.Value(source))) == "" {
			continue
		}
		_, ok := inline.(*ast.Link)
		return ok
	}
	return false
}

func validateLog(path string, raw []byte) []Finding {
	if !utf8.Valid(raw) {
		line, column := invalidUTF8Position(raw)
		return []Finding{newParseFinding(SeverityError, CodeInvalidUTF8, path, line, column, "log is not valid UTF-8")}
	}
	document := goldmark.DefaultParser().Parse(text.NewReader(raw))
	var findings []Finding
	for node := document.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok || heading.Level != 2 {
			continue
		}
		value := strings.TrimSpace(string(heading.Text(raw)))
		if !isISODate(value) {
			offset := lineStartOffset(raw, blockOffset(heading))
			line, column := bytePosition(raw, offset)
			findings = append(findings, reservedFinding(CodeMalformedLogDate, path, line, column, "level-2 log heading must be an ISO date (YYYY-MM-DD)"))
		}
	}
	return findings
}

func lineStartOffset(source []byte, offset int) int {
	if offset > len(source) {
		offset = len(source)
	}
	if newline := bytes.LastIndexByte(source[:offset], '\n'); newline >= 0 {
		return newline + 1
	}
	return 0
}

func isISODate(value string) bool {
	if len(value) != len("2006-01-02") {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func blockOffset(node ast.Node) int {
	if lines := node.Lines(); lines != nil && lines.Len() > 0 {
		return lines.At(0).Start
	}
	if child := node.FirstChild(); child != nil {
		return blockOffset(child)
	}
	return 0
}

func bytePosition(raw []byte, offset int) (line, column int) {
	line, column = 1, 1
	if offset > len(raw) {
		offset = len(raw)
	}
	for len(raw) > 0 && offset > 0 {
		r, size := utf8.DecodeRune(raw)
		if size > offset {
			break
		}
		if r == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
		raw, offset = raw[size:], offset-size
	}
	return line, column
}

func reservedFinding(code FindingCode, path string, line, column int, message string) Finding {
	return Finding{Severity: SeverityError, Code: code, Path: path, Line: position(line), Column: position(column), SpecSection: "4.1", Message: message}
}
