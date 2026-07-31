package okf

import "testing"

func TestConceptLifecycleTagAndLegacyAdvisories(t *testing.T) {
	source := []byte("---\n" +
		"type: Reference\n" +
		"status: unknown\n" +
		"stale_after: 2026-02-30\n" +
		"tags: one\n" +
		"timestamp: 2026-01-01T00:00:00Z\n" +
		"---\n# Definition\n\nText.\n\n# Citations\n\n* Legacy.\n")
	document, parseFindings, err := ParseDocument("concept.md", source)
	if err != nil || len(parseFindings) != 0 {
		t.Fatalf("ParseDocument = findings:%v error:%v", findingCodes(parseFindings), err)
	}
	findings := validateConceptAdvisories(document)
	assertFindingCodes(t, findings,
		CodeMalformedStatus, CodeMalformedStaleAfter, CodeMalformedTags,
		CodeLegacyTimestamp, CodeLegacyCitations,
	)
}

func TestGeneratedSuppressesLegacyTimestampFallbackWarning(t *testing.T) {
	source := []byte("---\ntype: Reference\ntimestamp: old\ngenerated: {by: human:one}\n---\n")
	document, _, err := ParseDocument("concept.md", source)
	if err != nil {
		t.Fatal(err)
	}
	if findings := validateConceptAdvisories(document); len(findings) != 0 {
		t.Fatalf("advisory findings = %v", findingCodes(findings))
	}
}

func TestDeclaredVersionSyntaxAndSupport(t *testing.T) {
	valid, declared := validateIndexWithVersion("index.md", []byte("---\nokf_version: 0.2\n---\n# Concepts\n\n* [One](one.md)\n"))
	if len(valid) != 0 || declared == nil || *declared != "0.2" {
		t.Fatalf("valid version = findings:%v declared:%v", findingCodes(valid), declared)
	}
	unsupported, declared := validateIndexWithVersion("index.md", []byte("---\nokf_version: 0.3\n---\n# Concepts\n\n* [One](one.md)\n"))
	assertFindingCodes(t, unsupported, CodeUnsupportedVersion)
	if declared == nil || *declared != "0.3" {
		t.Fatalf("unsupported declared version = %v", declared)
	}
	invalid, _ := validateIndexWithVersion("index.md", []byte("---\nokf_version: v2\n---\n# Concepts\n\n* [One](one.md)\n"))
	assertFindingCodes(t, invalid, CodeInvalidVersion)
}
