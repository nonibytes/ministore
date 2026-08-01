package okf

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

func validateProvenanceAndTrust(document Document) []Finding {
	metadata := document.Metadata()
	if metadata == nil {
		return nil
	}
	findings, sourceIDs := validateSources(document.Path, metadata)
	findings = append(findings, validateUsageWindow(document.Path, metadata, metadata.Root(), "usage_window")...)
	generatedAt, generatedFindings := validateGenerated(document.Path, metadata)
	findings = append(findings, generatedFindings...)
	latestVerified, verifiedFindings := validateVerified(document.Path, metadata)
	findings = append(findings, verifiedFindings...)
	if !generatedAt.IsZero() && !latestVerified.IsZero() && latestVerified.Before(generatedAt) {
		node, _ := metadata.Lookup("verified")
		findings = append(findings, advisoryFinding(CodeVerificationPredatesGen, document.Path, node, "latest verification predates generated.at", "5.2"))
	}
	findings = append(findings, unmatchedFootnoteFindings(document, sourceIDs)...)
	return findings
}

func validateSources(path string, metadata *Metadata) ([]Finding, map[string]struct{}) {
	node, exists := metadata.Lookup("sources")
	ids := make(map[string]struct{})
	if !exists {
		return nil, ids
	}
	node = ResolveAlias(node)
	var mappings []*yaml.Node
	var findings []Finding
	switch {
	case node != nil && node.Kind == yaml.MappingNode:
		mappings = []*yaml.Node{node}
		findings = append(findings, advisoryFinding(CodeMalformedSources, path, node, "sources should be a sequence; a mapping is consumed as one entry", "5.1"))
	case node != nil && node.Kind == yaml.SequenceNode:
		for _, entry := range node.Content {
			entry = ResolveAlias(entry)
			if entry == nil || entry.Kind != yaml.MappingNode {
				findings = append(findings, advisoryFinding(CodeMalformedSources, path, entry, "sources entry must be a mapping", "5.1"))
				continue
			}
			mappings = append(mappings, entry)
		}
	default:
		findings = append(findings, advisoryFinding(CodeMalformedSources, path, node, "sources must be a sequence of mappings", "5.1"))
		return findings, ids
	}
	for _, source := range mappings {
		resource, resourceExists := MappingLookup(source, "resource")
		resourceValue, resourceValid := NodeString(resource)
		if !resourceExists || !resourceValid || strings.TrimSpace(resourceValue) == "" {
			target := source
			if resourceExists {
				target = resource
			}
			findings = append(findings, advisoryFinding(CodeSourceMissingResource, path, target, "source entry requires a non-empty resource", "5.1"))
		}
		if idNode, ok := MappingLookup(source, "id"); ok {
			id, valid := NodeString(idNode)
			if !valid {
				findings = append(findings, advisoryFinding(CodeMalformedSources, path, idNode, "source id must be a string", "5.1"))
			} else if id = strings.TrimSpace(id); id != "" {
				if _, duplicate := ids[id]; duplicate {
					findings = append(findings, advisoryFinding(CodeDuplicateSourceID, path, idNode, "source id is duplicated", "5.1"))
				} else {
					ids[id] = struct{}{}
				}
			}
		}
		if title, ok := MappingLookup(source, "title"); ok {
			if _, valid := NodeString(title); !valid {
				findings = append(findings, advisoryFinding(CodeMalformedSources, path, title, "source title must be a string", "5.1"))
			}
		}
		if author, ok := MappingLookup(source, "author"); ok {
			value, valid := NodeString(author)
			if !valid {
				findings = append(findings, advisoryFinding(CodeMalformedCredibility, path, author, "source author must be a string", "5.1"))
			} else if !validActor(value) {
				findings = append(findings, advisoryFinding(CodeMalformedActor, path, author, "source author does not follow the actor convention", "7"))
			}
		}
		if count, ok := MappingLookup(source, "usage_count"); ok && !validUsageCount(count) {
			findings = append(findings, advisoryFinding(CodeMalformedCredibility, path, count, "source usage_count must be a non-negative number", "5.1"))
		}
		if modified, ok := MappingLookup(source, "last_modified"); ok {
			value, valid := NodeDate(modified)
			if !valid || !isISODate(value) {
				findings = append(findings, advisoryFinding(CodeMalformedCredibility, path, modified, "source last_modified must be an ISO date", "5.1"))
			}
		}
		findings = append(findings, validateUsageWindow(path, metadata, source, "usage_window")...)
	}
	return findings, ids
}

func validateUsageWindow(path string, metadata *Metadata, mapping *yaml.Node, key string) []Finding {
	var node *yaml.Node
	var exists bool
	if mapping == metadata.Root() {
		node, exists = metadata.Lookup(key)
	} else {
		node, exists = MappingLookup(mapping, key)
	}
	if !exists {
		return nil
	}
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return []Finding{advisoryFinding(CodeMalformedUsageWindow, path, node, "usage_window must contain from and to dates", "5.1")}
	}
	fromNode, fromExists := MappingLookup(node, "from")
	toNode, toExists := MappingLookup(node, "to")
	from, fromValid := NodeDate(fromNode)
	to, toValid := NodeDate(toNode)
	if !fromExists || !toExists || !fromValid || !toValid || !isISODate(from) || !isISODate(to) || from > to {
		return []Finding{advisoryFinding(CodeMalformedUsageWindow, path, node, "usage_window must contain an ordered ISO date range", "5.1")}
	}
	return nil
}

func validateGenerated(path string, metadata *Metadata) (time.Time, []Finding) {
	node, exists := metadata.Lookup("generated")
	if !exists {
		return time.Time{}, nil
	}
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return time.Time{}, []Finding{advisoryFinding(CodeMalformedGenerated, path, node, "generated must be a mapping with a non-empty by actor", "5.2")}
	}
	var findings []Finding
	byNode, byExists := MappingLookup(node, "by")
	by, byValid := NodeString(byNode)
	if !byExists || !byValid || strings.TrimSpace(by) == "" {
		findings = append(findings, advisoryFinding(CodeMalformedGenerated, path, byNodeOrMapping(byNode, node), "generated requires a non-empty by actor", "5.2"))
	} else if !validActor(by) {
		findings = append(findings, advisoryFinding(CodeMalformedActor, path, byNode, "generated.by does not follow the actor convention", "7"))
	}
	var at time.Time
	if atNode, ok := MappingLookup(node, "at"); ok {
		value, valid := NodeDate(atNode)
		if valid {
			at, _ = time.Parse(time.RFC3339, value)
		}
		if at.IsZero() {
			findings = append(findings, advisoryFinding(CodeMalformedGeneratedAt, path, atNode, "generated.at must be an ISO 8601 datetime", "5.2"))
		}
	}
	return at, findings
}

func validateVerified(path string, metadata *Metadata) (time.Time, []Finding) {
	node, exists := metadata.Lookup("verified")
	if !exists {
		return time.Time{}, nil
	}
	node = ResolveAlias(node)
	var events []*yaml.Node
	var findings []Finding
	if node != nil && node.Kind == yaml.MappingNode {
		events = []*yaml.Node{node}
	} else if node != nil && node.Kind == yaml.SequenceNode {
		for _, event := range node.Content {
			event = ResolveAlias(event)
			if event == nil || event.Kind != yaml.MappingNode {
				findings = append(findings, advisoryFinding(CodeMalformedVerified, path, event, "verified entries must be mappings", "5.2"))
				continue
			}
			events = append(events, event)
		}
	} else {
		return time.Time{}, []Finding{advisoryFinding(CodeMalformedVerified, path, node, "verified must be a mapping or sequence of mappings", "5.2")}
	}
	var latest time.Time
	for _, event := range events {
		byNode, byExists := MappingLookup(event, "by")
		atNode, atExists := MappingLookup(event, "at")
		by, byValid := NodeString(byNode)
		atValue, atValid := NodeDate(atNode)
		parsedAt := time.Time{}
		if atValid {
			parsedAt, _ = time.Parse(time.RFC3339, atValue)
		}
		if !byExists || !atExists || !byValid || strings.TrimSpace(by) == "" || parsedAt.IsZero() {
			findings = append(findings, advisoryFinding(CodeMalformedVerification, path, event, "verification event requires valid by and at values", "5.2"))
		} else {
			if parsedAt.After(latest) {
				latest = parsedAt
			}
			if !validActor(by) {
				findings = append(findings, advisoryFinding(CodeMalformedActor, path, byNode, "verified.by does not follow the actor convention", "7"))
			}
		}
	}
	return latest, findings
}

func validActor(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false
	}
	if left, right, found := strings.Cut(value, ":"); found {
		return left != "" && right != ""
	}
	left, right, found := strings.Cut(value, "/")
	return found && left != "" && right != ""
}

func validUsageCount(node *yaml.Node) bool {
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.ScalarNode || (node.ShortTag() != "!!int" && node.ShortTag() != "!!float") {
		return false
	}
	value, err := strconv.ParseFloat(strings.ReplaceAll(node.Value, "_", ""), 64)
	return err == nil && value >= 0
}

func byNodeOrMapping(node, mapping *yaml.Node) *yaml.Node {
	if node != nil {
		return node
	}
	return mapping
}

func unmatchedFootnoteFindings(document Document, ids map[string]struct{}) []Finding {
	body := document.Body()
	if body == nil {
		return nil
	}
	parser := goldmark.New(goldmark.WithExtensions(extension.Footnote)).Parser()
	root := parser.Parse(text.NewReader(body))
	var findings []Finding
	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		footnote, ok := node.(*extensionast.Footnote)
		if !ok {
			return ast.WalkContinue, nil
		}
		label := string(footnote.Ref)
		if _, exists := ids[label]; !exists {
			offset := lineStartOffset(body, blockOffset(footnote))
			line, column := bytePosition(document.Raw(), document.body.start+offset)
			findings = append(findings, Finding{Severity: SeverityWarning, Code: CodeUnmatchedFootnote, Path: document.Path, Line: position(line), Column: position(column), SpecSection: "5.1", Message: "footnote label has no matching source id"})
		}
		return ast.WalkContinue, nil
	})
	return findings
}

func validateAttestedComputation(document Document) []Finding {
	metadata := document.Metadata()
	if metadata == nil || metadataString(metadata, "type") != "Attested Computation" {
		return nil
	}
	var findings []Finding
	if runtime, ok := metadata.Lookup("runtime"); !ok {
		findings = append(findings, advisoryFinding(CodeMissingRuntime, document.Path, metadata.Root(), "Attested Computation requires runtime", "10.2"))
	} else if value, valid := NodeString(runtime); !valid || strings.TrimSpace(value) == "" {
		findings = append(findings, advisoryFinding(CodeMissingRuntime, document.Path, runtime, "Attested Computation requires a non-empty runtime", "10.2"))
	}
	if parameters, ok := metadata.Lookup("parameters"); ok {
		parameters = ResolveAlias(parameters)
		if parameters == nil || parameters.Kind != yaml.SequenceNode {
			findings = append(findings, advisoryFinding(CodeMalformedParameters, document.Path, parameters, "parameters must be a sequence", "10.2"))
		} else {
			for _, parameter := range parameters.Content {
				parameter = ResolveAlias(parameter)
				if !validParameter(parameter) {
					findings = append(findings, advisoryFinding(CodeMalformedParameters, document.Path, parameter, "parameter requires string name/type and boolean required", "10.2"))
				}
			}
		}
	}
	computationNode, hasFile := metadata.Lookup("computation")
	if hasFile {
		value, valid := NodeString(computationNode)
		hasFile = valid && strings.TrimSpace(value) != ""
	}
	hasInline := hasInlineComputation(document.Body())
	if hasFile == hasInline {
		findings = append(findings, advisoryFinding(CodeMalformedComputation, document.Path, byNodeOrMapping(computationNode, metadata.Root()), "provide exactly one file or inline computation", "10.3"))
	}
	findings = append(findings, validateExecutor(document.Path, metadata)...)
	findings = append(findings, validateAttester(document.Path, metadata)...)
	return findings
}

func metadataString(metadata *Metadata, key string) string {
	value, _ := metadata.String(key)
	return value
}

func validParameter(node *yaml.Node) bool {
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	name, nameOK := MappingLookup(node, "name")
	typeNode, typeOK := MappingLookup(node, "type")
	required, requiredOK := MappingLookup(node, "required")
	nameValue, nameValid := NodeString(name)
	typeValue, typeValid := NodeString(typeNode)
	required = ResolveAlias(required)
	return nameOK && typeOK && requiredOK && nameValid && typeValid && strings.TrimSpace(nameValue) != "" && strings.TrimSpace(typeValue) != "" && required != nil && required.Kind == yaml.ScalarNode && required.ShortTag() == "!!bool"
}

func hasInlineComputation(body []byte) bool {
	if body == nil {
		return false
	}
	root := goldmark.DefaultParser().Parse(text.NewReader(body))
	inSection := false
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		if heading, ok := node.(*ast.Heading); ok && heading.Level == 1 {
			inSection = strings.TrimSpace(string(heading.Text(body))) == "Computation"
			continue
		}
		if inSection {
			if _, ok := node.(*ast.FencedCodeBlock); ok {
				return true
			}
		}
	}
	return false
}

func validateExecutor(path string, metadata *Metadata) []Finding {
	node, exists := metadata.Lookup("executor")
	if !exists {
		return nil
	}
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return []Finding{advisoryFinding(CodeMalformedExecutor, path, node, "executor must be a mapping", "10.2")}
	}
	resource, resourceOK := MappingLookup(node, "resource")
	resourceValue, resourceValid := NodeString(resource)
	if !resourceOK || !resourceValid || strings.TrimSpace(resourceValue) == "" {
		return []Finding{advisoryFinding(CodeMalformedExecutor, path, node, "executor requires a non-empty resource", "10.2")}
	}
	if receipt, ok := MappingLookup(node, "receipt"); ok {
		receipt = ResolveAlias(receipt)
		if receipt == nil || receipt.Kind != yaml.SequenceNode {
			return []Finding{advisoryFinding(CodeMalformedExecutor, path, receipt, "executor receipt must be a sequence of strings", "10.2")}
		}
		for _, field := range receipt.Content {
			value, valid := NodeString(field)
			if !valid || strings.TrimSpace(value) == "" {
				return []Finding{advisoryFinding(CodeMalformedExecutor, path, field, "executor receipt fields must be non-empty strings", "10.2")}
			}
		}
	}
	return nil
}

func validateAttester(path string, metadata *Metadata) []Finding {
	node, exists := metadata.Lookup("attester")
	if !exists {
		return nil
	}
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return []Finding{advisoryFinding(CodeMalformedAttester, path, node, "attester must be a mapping with a resource", "10.2")}
	}
	resource, ok := MappingLookup(node, "resource")
	value, valid := NodeString(resource)
	if !ok || !valid || strings.TrimSpace(value) == "" {
		return []Finding{advisoryFinding(CodeMalformedAttester, path, node, "attester requires a non-empty resource", "10.2")}
	}
	return nil
}
