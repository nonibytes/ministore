package okf

import "testing"

func TestReservedIndexCommonMarkStructure(t *testing.T) {
	valid := []struct {
		path   string
		source string
	}{
		{path: "index.md", source: "---\nokf_version: 0.2\n---\n# Concepts\n\n* [One](one.md) detail\n"},
		{path: "index.md", source: "# Concepts\n\n1. [One][one]\n\n[one]: one.md\n"},
	}
	for _, test := range valid {
		if findings := validateIndex(test.path, []byte(test.source)); len(findings) != 0 {
			t.Fatalf("valid index findings = %v", findingCodes(findings))
		}
	}

	fenced := validateIndex("index.md", []byte("```md\n# Fake\n* [Fake](x)\n```\n"))
	assertFindingCodes(t, fenced, CodeMalformedIndex)
	nested := validateIndex("nested/index.md", []byte("---\nokf_version: 0.2\n---\n# Concepts\n\n* [One](one.md)\n"))
	assertFindingCodes(t, nested, CodeNestedIndexFrontmatter)
	linkLast := validateIndex("index.md", []byte("# Concepts\n\n* Description [One](one.md)\n"))
	assertFindingCodes(t, linkLast, CodeIndexEntryNotLinkFirst)
}

func TestReservedLogDatesUseCommonMarkAST(t *testing.T) {
	if findings := validateLog("log.md", []byte("# Log\n\n## 2026-06-25\n\n* Added.\n")); len(findings) != 0 {
		t.Fatalf("valid log findings = %v", findingCodes(findings))
	}
	if findings := validateLog("log.md", []byte("```md\n## Not a date\n```\n")); len(findings) != 0 {
		t.Fatalf("fenced heading findings = %v", findingCodes(findings))
	}
	invalid := validateLog("log.md", []byte("## 2026-02-30\n"))
	assertFindingCodes(t, invalid, CodeMalformedLogDate)
}
