package okf

import (
	"bytes"
	"io"

	"gopkg.in/yaml.v3"
)

var recognizedTopLevelKeys = map[string]struct{}{
	"type": {}, "title": {}, "description": {}, "resource": {}, "tags": {},
	"sources": {}, "usage_window": {}, "generated": {}, "verified": {},
	"status": {}, "stale_after": {}, "timestamp": {}, "runtime": {},
	"parameters": {}, "computation": {}, "executor": {}, "attester": {},
	"okf_version": {},
}

// Metadata retains the YAML node tree and indexes the first occurrence of each
// string top-level key. Arbitrary extension values remain nodes and are never
// flattened into MiniStore fields.
type Metadata struct {
	root  *yaml.Node
	first map[string]*yaml.Node
}

// Root returns the YAML mapping node.
func (m *Metadata) Root() *yaml.Node {
	if m == nil {
		return nil
	}
	return m.root
}

// Lookup returns the first value for an exact string top-level key.
func (m *Metadata) Lookup(key string) (*yaml.Node, bool) {
	if m == nil {
		return nil, false
	}
	node, ok := m.first[key]
	return node, ok
}

// String returns a recognized YAML string scalar without coercing numbers,
// booleans, timestamps, mappings, or sequences.
func (m *Metadata) String(key string) (string, bool) {
	node, ok := m.Lookup(key)
	if !ok {
		return "", false
	}
	return NodeString(node)
}

// Date returns the original lexical value of a YAML string or timestamp scalar.
func (m *Metadata) Date(key string) (string, bool) {
	node, ok := m.Lookup(key)
	if !ok {
		return "", false
	}
	return NodeDate(node)
}

// CollectionForm describes the source container used for a permissive accessor.
type CollectionForm uint8

const (
	CollectionMissing CollectionForm = iota
	CollectionScalar
	CollectionSequence
	CollectionMapping
	CollectionOther
)

// Strings returns string values from either a scalar or sequence. valid is false
// when any sequence entry is not a YAML string scalar.
func (m *Metadata) Strings(key string) (values []string, form CollectionForm, valid bool) {
	node, ok := m.Lookup(key)
	if !ok {
		return nil, CollectionMissing, false
	}
	node = ResolveAlias(node)
	if value, ok := NodeString(node); ok {
		return []string{value}, CollectionScalar, true
	}
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, nodeCollectionForm(node), false
	}
	values = make([]string, 0, len(node.Content))
	valid = true
	for _, item := range node.Content {
		value, ok := NodeString(item)
		if !ok {
			valid = false
			continue
		}
		values = append(values, value)
	}
	return values, CollectionSequence, valid
}

// Mappings returns a mapping as one entry or every mapping in a sequence. valid
// is false when any sequence entry is not a mapping.
func (m *Metadata) Mappings(key string) (values []*yaml.Node, form CollectionForm, valid bool) {
	node, ok := m.Lookup(key)
	if !ok {
		return nil, CollectionMissing, false
	}
	node = ResolveAlias(node)
	if node != nil && node.Kind == yaml.MappingNode {
		return []*yaml.Node{node}, CollectionMapping, true
	}
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil, nodeCollectionForm(node), false
	}
	values = make([]*yaml.Node, 0, len(node.Content))
	valid = true
	for _, item := range node.Content {
		item = ResolveAlias(item)
		if item == nil || item.Kind != yaml.MappingNode {
			valid = false
			continue
		}
		values = append(values, item)
	}
	return values, CollectionSequence, valid
}

// MappingLookup returns the first exact string key from a mapping node.
func MappingLookup(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	mapping = ResolveAlias(mapping)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		name, ok := NodeString(mapping.Content[i])
		if ok && name == key {
			return mapping.Content[i+1], true
		}
	}
	return nil, false
}

// ResolveAlias follows YAML aliases for recognized-field access.
func ResolveAlias(node *yaml.Node) *yaml.Node {
	for node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

// NodeString returns a scalar only when YAML classified it as a string.
func NodeString(node *yaml.Node) (string, bool) {
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.ScalarNode || node.ShortTag() != "!!str" {
		return "", false
	}
	return node.Value, true
}

// NodeDate returns a string or timestamp scalar in its original lexical form.
func NodeDate(node *yaml.Node) (string, bool) {
	node = ResolveAlias(node)
	if node == nil || node.Kind != yaml.ScalarNode {
		return "", false
	}
	tag := node.ShortTag()
	if tag != "!!str" && tag != "!!timestamp" {
		return "", false
	}
	return node.Value, true
}

func nodeCollectionForm(node *yaml.Node) CollectionForm {
	node = ResolveAlias(node)
	if node == nil {
		return CollectionOther
	}
	switch node.Kind {
	case yaml.ScalarNode:
		return CollectionScalar
	case yaml.SequenceNode:
		return CollectionSequence
	case yaml.MappingNode:
		return CollectionMapping
	default:
		return CollectionOther
	}
}

func parseMetadata(path string, frontmatter []byte) (*Metadata, []Finding) {
	decoder := yaml.NewDecoder(bytes.NewReader(frontmatter))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return nil, []Finding{newParseFinding(
				SeverityError, CodeFrontmatterNotMapping, path, 2, 1,
				"frontmatter root is not a mapping",
			)}
		}
		return nil, []Finding{{
			Severity: SeverityError, Code: CodeInvalidYAML, Path: path,
			SpecSection: "4.1", Message: "frontmatter is not parseable YAML: " + err.Error(),
		}}
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, []Finding{{
				Severity: SeverityError, Code: CodeInvalidYAML, Path: path,
				SpecSection: "4.1", Message: "frontmatter contains multiple YAML documents",
			}}
		}
		return nil, []Finding{{
			Severity: SeverityError, Code: CodeInvalidYAML, Path: path,
			SpecSection: "4.1", Message: "frontmatter is not parseable YAML: " + err.Error(),
		}}
	}

	if len(document.Content) != 1 {
		return nil, []Finding{newParseFinding(
			SeverityError, CodeFrontmatterNotMapping, path, 2, 1,
			"frontmatter root is not a mapping",
		)}
	}
	root := ResolveAlias(document.Content[0])
	if root == nil || root.Kind != yaml.MappingNode {
		line, column := 2, 1
		if root != nil {
			line, column = root.Line+1, root.Column
		}
		return nil, []Finding{newParseFinding(
			SeverityError, CodeFrontmatterNotMapping, path, line, column,
			"frontmatter root is not a mapping",
		)}
	}

	metadata := &Metadata{root: root, first: make(map[string]*yaml.Node)}
	var findings []Finding
	for i := 0; i+1 < len(root.Content); i += 2 {
		keyNode, valueNode := root.Content[i], root.Content[i+1]
		key, ok := NodeString(keyNode)
		if !ok {
			continue
		}
		if _, exists := metadata.first[key]; exists {
			if _, recognized := recognizedTopLevelKeys[key]; recognized {
				findings = append(findings, duplicateKeyFinding(
					path, key, keyNode.Line+1, keyNode.Column,
				))
			}
			continue
		}
		metadata.first[key] = valueNode
	}
	return metadata, findings
}
