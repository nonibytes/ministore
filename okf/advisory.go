package okf

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

func validateConceptAdvisories(document Document) []Finding {
	metadata := document.Metadata()
	if metadata == nil {
		return nil
	}
	var findings []Finding

	if node, exists := metadata.Lookup("status"); exists {
		value, valid := NodeString(node)
		if !valid || (value != "draft" && value != "stable" && value != "deprecated") {
			findings = append(findings, advisoryFinding(CodeMalformedStatus, document.Path, node, "status should be draft, stable, or deprecated", "5.4"))
		}
	}
	if node, exists := metadata.Lookup("stale_after"); exists {
		value, valid := NodeDate(node)
		if !valid || !isISODate(value) {
			findings = append(findings, advisoryFinding(CodeMalformedStaleAfter, document.Path, node, "stale_after must be an ISO date (YYYY-MM-DD)", "5.5"))
		}
	}
	if node, exists := metadata.Lookup("tags"); exists {
		_, form, valid := metadata.Strings("tags")
		if form != CollectionSequence || !valid {
			findings = append(findings, advisoryFinding(CodeMalformedTags, document.Path, node, "tags should be a sequence of strings", "4.1"))
		}
	}
	if node, legacy := metadata.Lookup("timestamp"); legacy {
		if _, generated := metadata.Lookup("generated"); !generated {
			findings = append(findings, advisoryFinding(CodeLegacyTimestamp, document.Path, node, "legacy timestamp is used as the generated.at fallback", "13.1"))
		}
	}
	findings = append(findings, legacyCitationsFindings(document)...)
	return findings
}

func legacyCitationsFindings(document Document) []Finding {
	body := document.Body()
	if body == nil {
		return nil
	}
	parsed := goldmark.DefaultParser().Parse(text.NewReader(body))
	for node := parsed.FirstChild(); node != nil; node = node.NextSibling() {
		heading, ok := node.(*ast.Heading)
		if !ok || heading.Level != 1 || strings.TrimSpace(string(heading.Text(body))) != "Citations" {
			continue
		}
		offset := lineStartOffset(body, blockOffset(heading))
		line, column := bytePosition(document.Raw(), document.body.start+offset)
		return []Finding{{
			Severity: SeverityWarning, Code: CodeLegacyCitations, Path: document.Path,
			Line: position(line), Column: position(column), SpecSection: "13.1",
			Message: "legacy Citations section is used as a provenance fallback",
		}}
	}
	return nil
}

func advisoryFinding(code FindingCode, path string, node *yaml.Node, message, section string) Finding {
	line, column := yamlNodePosition(node)
	return Finding{
		Severity: SeverityWarning, Code: code, Path: path,
		Line: position(line), Column: position(column), SpecSection: section,
		Message: message,
	}
}

func yamlNodePosition(node *yaml.Node) (int, int) {
	node = ResolveAlias(node)
	if node == nil {
		return 1, 1
	}
	return node.Line + 1, node.Column
}
