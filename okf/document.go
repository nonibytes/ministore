package okf

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

var utf8BOM = []byte{0xef, 0xbb, 0xbf}

// Document retains one complete concept document and byte offsets into it.
// Callers must treat returned byte slices as read-only parser-owned storage.
type Document struct {
	Path     string
	raw      []byte
	front    byteRange
	body     byteRange
	metadata *Metadata
}

type byteRange struct {
	start int
	end   int
	valid bool
}

// Raw returns the exact input bytes, including a BOM and original line endings.
func (d Document) Raw() []byte {
	return d.raw
}

// FrontmatterYAML returns the exact bytes between the two delimiter lines.
func (d Document) FrontmatterYAML() []byte {
	if !d.front.valid {
		return nil
	}
	return d.raw[d.front.start:d.front.end]
}

// Body returns the exact bytes after the closing delimiter and its line ending.
func (d Document) Body() []byte {
	if !d.body.valid {
		return nil
	}
	return d.raw[d.body.start:d.body.end]
}

// Metadata returns the parsed frontmatter mapping when YAML parsing succeeded.
func (d Document) Metadata() *Metadata {
	return d.metadata
}

// ParseDocument splits and parses one OKF concept without normalizing any source
// bytes. Format defects are returned as findings; error is reserved for
// operational failures and is nil for this in-memory operation.
func ParseDocument(path string, raw []byte) (Document, []Finding, error) {
	doc := Document{Path: path, raw: bytes.Clone(raw)}
	if !utf8.Valid(doc.raw) {
		line, column := invalidUTF8Position(doc.raw)
		return doc, []Finding{newParseFinding(
			SeverityError, CodeInvalidUTF8, path, line, column,
			"concept is not valid UTF-8",
		)}, nil
	}

	var findings []Finding
	contentStart := 0
	if bytes.HasPrefix(doc.raw, utf8BOM) {
		contentStart = len(utf8BOM)
		findings = append(findings, newParseFinding(
			SeverityWarning, CodeUTF8BOM, path, 1, 1,
			"document begins with a UTF-8 BOM",
		))
	}

	openingContentEnd, openingLineEnd := physicalLine(doc.raw, contentStart)
	opening := string(doc.raw[contentStart:openingContentEnd])
	if strings.TrimSpace(opening) != "---" {
		findings = append(findings, newParseFinding(
			SeverityError, CodeMissingOpeningDelimiter, path, 1, 1,
			"concept has no opening frontmatter delimiter",
		))
		return doc, findings, nil
	}
	if opening != "---" {
		findings = append(findings, newParseFinding(
			SeverityWarning, CodeDelimiterWhitespace, path, 1, 1,
			"opening frontmatter delimiter contains surrounding whitespace",
		))
	}

	frontStart := openingLineEnd
	lineStart := openingLineEnd
	for lineStart < len(doc.raw) {
		contentEnd, lineEnd := physicalLine(doc.raw, lineStart)
		if bytes.Equal(doc.raw[lineStart:contentEnd], []byte("---")) {
			doc.front = byteRange{start: frontStart, end: lineStart, valid: true}
			doc.body = byteRange{start: lineEnd, end: len(doc.raw), valid: true}
			metadata, yamlFindings := parseMetadata(path, doc.FrontmatterYAML())
			doc.metadata = metadata
			findings = append(findings, yamlFindings...)
			return doc, findings, nil
		}
		if lineEnd == len(doc.raw) {
			break
		}
		lineStart = lineEnd
	}

	findings = append(findings, newParseFinding(
		SeverityError, CodeMissingClosingDelimiter, path, 1, 1,
		"concept has no closing frontmatter delimiter",
	))
	return doc, findings, nil
}

func physicalLine(raw []byte, start int) (contentEnd, lineEnd int) {
	relativeNewline := bytes.IndexByte(raw[start:], '\n')
	if relativeNewline < 0 {
		return len(raw), len(raw)
	}
	newline := start + relativeNewline
	contentEnd = newline
	if newline > start && raw[newline-1] == '\r' {
		contentEnd--
	}
	return contentEnd, newline + 1
}

func invalidUTF8Position(raw []byte) (line, column int) {
	line, column = 1, 1
	for len(raw) > 0 {
		r, size := utf8.DecodeRune(raw)
		if r == utf8.RuneError && size == 1 {
			return line, column
		}
		if r == '\n' {
			line, column = line+1, 1
		} else {
			column++
		}
		raw = raw[size:]
	}
	return line, column
}

func newParseFinding(severity Severity, code FindingCode, path string, line, column int, message string) Finding {
	return Finding{
		Severity: severity, Code: code, Path: path,
		Line: position(line), Column: position(column),
		SpecSection: "4.1", Message: message,
	}
}

func duplicateKeyFinding(path, key string, line, column int) Finding {
	return newParseFinding(
		SeverityWarning, CodeDuplicateKey, path, line, column,
		fmt.Sprintf("recognized top-level key %q occurs more than once", key),
	)
}
