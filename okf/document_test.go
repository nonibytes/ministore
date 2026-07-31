package okf

import (
	"bytes"
	"testing"
)

func TestParseDocumentPreservesBytesAndOffsets(t *testing.T) {
	tests := []struct {
		name  string
		raw   []byte
		front []byte
		body  []byte
		codes []FindingCode
	}{
		{
			name:  "LF",
			raw:   []byte("---\ntype: Reference\n---\nbody  \n---\n"),
			front: []byte("type: Reference\n"),
			body:  []byte("body  \n---\n"),
		},
		{
			name:  "CRLF without final newline",
			raw:   []byte("---\r\ntype: Reference\r\n---\r\nbody  "),
			front: []byte("type: Reference\r\n"),
			body:  []byte("body  "),
		},
		{
			name:  "BOM and tolerated opening whitespace",
			raw:   append(append(bytes.Clone(utf8BOM), []byte(" \t--- \r\n")...), []byte("type: Reference\r\n---\r\n")...),
			front: []byte("type: Reference\r\n"),
			body:  []byte{},
			codes: []FindingCode{CodeUTF8BOM, CodeDelimiterWhitespace},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, findings, err := ParseDocument("concept.md", test.raw)
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			if !bytes.Equal(document.Raw(), test.raw) {
				t.Fatalf("raw bytes changed: %q", document.Raw())
			}
			if !bytes.Equal(document.FrontmatterYAML(), test.front) {
				t.Fatalf("frontmatter = %q, want %q", document.FrontmatterYAML(), test.front)
			}
			if !bytes.Equal(document.Body(), test.body) {
				t.Fatalf("body = %q, want %q", document.Body(), test.body)
			}
			assertFindingCodes(t, findings, test.codes...)
		})
	}
}

func TestParseDocumentFormatFindings(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		code FindingCode
	}{
		{name: "invalid UTF-8", raw: []byte{'-', '-', '-', '\n', 0xff}, code: CodeInvalidUTF8},
		{name: "missing opening", raw: []byte("type: Reference\n"), code: CodeMissingOpeningDelimiter},
		{name: "missing closing", raw: []byte("---\ntype: Reference\n"), code: CodeMissingClosingDelimiter},
		{name: "invalid YAML", raw: []byte("---\ntype: [unterminated\n---\n"), code: CodeInvalidYAML},
		{name: "empty YAML", raw: []byte("---\n---\n"), code: CodeFrontmatterNotMapping},
		{name: "sequence root", raw: []byte("---\n- Reference\n---\n"), code: CodeFrontmatterNotMapping},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, findings, err := ParseDocument("concept.md", test.raw)
			if err != nil {
				t.Fatalf("ParseDocument: %v", err)
			}
			assertFindingCodes(t, findings, test.code)
		})
	}
}

func TestMetadataUsesFirstRecognizedKeyAndTypedAccessors(t *testing.T) {
	raw := []byte("---\n" +
		"type: First\n" +
		"type: Second\n" +
		"title: 2026-07-31\n" +
		"stale_after: 2026-07-31\n" +
		"tags: &tags [one, two]\n" +
		"x-alias: *tags\n" +
		"verified: {by: 'human:reviewer', at: 2026-07-31T12:00:00Z}\n" +
		"x-extension: !vendor {nested: true}\n" +
		"---\nbody\n")
	document, findings, err := ParseDocument("concept.md", raw)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	assertFindingCodes(t, findings, CodeDuplicateKey)
	metadata := document.Metadata()
	if got, ok := metadata.String("type"); !ok || got != "First" {
		t.Fatalf("type = %q, %v", got, ok)
	}
	if _, ok := metadata.String("title"); ok {
		t.Fatal("unquoted timestamp was coerced to a string")
	}
	if got, ok := metadata.Date("stale_after"); !ok || got != "2026-07-31" {
		t.Fatalf("stale_after = %q, %v", got, ok)
	}
	if got, form, valid := metadata.Strings("tags"); !valid || form != CollectionSequence || len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("tags = %q, %v, %v", got, form, valid)
	}
	if got, form, valid := metadata.Mappings("verified"); !valid || form != CollectionMapping || len(got) != 1 {
		t.Fatalf("verified mappings = %d, %v, %v", len(got), form, valid)
	}
	if _, ok := metadata.Lookup("x-extension"); !ok {
		t.Fatal("unknown extension node was discarded")
	}
}

func TestParseDocumentOwnsInput(t *testing.T) {
	raw := []byte("---\ntype: Reference\n---\nbody")
	document, _, err := ParseDocument("concept.md", raw)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	raw[0] = 'x'
	if document.Raw()[0] != '-' {
		t.Fatal("document aliases caller-owned input")
	}
}

func TestRecursiveAliasInUnknownExtensionIsPreservedWithoutExpansion(t *testing.T) {
	raw := []byte("---\ntype: Reference\nx-recursive: &value [*value]\n---\n")
	document, findings, err := ParseDocument("concept.md", raw)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v", findingCodes(findings))
	}
	if _, ok := document.Metadata().Lookup("x-recursive"); !ok {
		t.Fatal("recursive unknown extension was discarded")
	}
}

func TestYAMLScalarClassification(t *testing.T) {
	raw := []byte("---\n" +
		"type: Reference\n" +
		"title: 2026-7-1T02:03:04Z\n" +
		"description: \"2026-7-1T02:03:04Z\"\n" +
		"resource: 42\n" +
		"---\n")
	document, findings, err := ParseDocument("concept.md", raw)
	if err != nil {
		t.Fatalf("ParseDocument: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %v", findingCodes(findings))
	}
	metadata := document.Metadata()
	if _, ok := metadata.String("title"); ok {
		t.Fatal("unquoted timestamp classified as a string")
	}
	if got, ok := metadata.Date("title"); !ok || got != "2026-7-1T02:03:04Z" {
		t.Fatalf("title date = %q, %v", got, ok)
	}
	if got, ok := metadata.String("description"); !ok || got != "2026-7-1T02:03:04Z" {
		t.Fatalf("description = %q, %v", got, ok)
	}
	if _, ok := metadata.String("resource"); ok {
		t.Fatal("integer classified as a string")
	}
}

func FuzzParseDocument(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("---\ntype: Reference\n---\nbody"),
		[]byte("---\r\ntype: Reference\r\n---\r\n"),
		[]byte("---\na: &a [*a]\n---\n"),
		{0xff, 0x00, '\n'},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw []byte) {
		document, _, err := ParseDocument("fuzz.md", raw)
		if err != nil {
			return
		}
		if !bytes.Equal(document.Raw(), raw) {
			t.Fatal("raw bytes changed")
		}
	})
}

func assertFindingCodes(t *testing.T, findings []Finding, want ...FindingCode) {
	t.Helper()
	if len(findings) != len(want) {
		t.Fatalf("finding codes = %v, want %v", findingCodes(findings), want)
	}
	for i := range want {
		if findings[i].Code != want[i] {
			t.Fatalf("finding codes = %v, want %v", findingCodes(findings), want)
		}
	}
}

func findingCodes(findings []Finding) []FindingCode {
	codes := make([]FindingCode, len(findings))
	for i := range findings {
		codes[i] = findings[i].Code
	}
	return codes
}
