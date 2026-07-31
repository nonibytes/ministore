package okf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type normalizedValidation struct {
	Fixture  string              `json:"fixture"`
	Concepts int                 `json:"concepts"`
	Errors   int                 `json:"errors"`
	Warnings int                 `json:"warnings"`
	Findings []normalizedFinding `json:"findings"`
}

type normalizedFinding struct {
	Severity    Severity    `json:"severity"`
	Code        FindingCode `json:"code"`
	Path        string      `json:"path"`
	Line        *int        `json:"line,omitempty"`
	Column      *int        `json:"column,omitempty"`
	SpecSection string      `json:"spec_section,omitempty"`
}

func TestBaseValidationGolden(t *testing.T) {
	root := filepath.Join("..", "testdata", "okf", "v0.2")
	golden, err := os.ReadFile(filepath.Join(root, "expected", "base-validation.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(bytes.TrimSpace(golden), []byte{'\n'}) {
		var expected normalizedValidation
		if err := json.Unmarshal(line, &expected); err != nil {
			t.Fatalf("decode golden line: %v", err)
		}
		var findings []Finding
		summary, err := ValidateBundle(context.Background(), filepath.Join(root, filepath.FromSlash(expected.Fixture)), ValidateOptions{}, func(finding Finding) error {
			findings = append(findings, finding)
			return nil
		})
		if err != nil {
			t.Fatalf("ValidateBundle(%s): %v", expected.Fixture, err)
		}
		actual := normalizedValidation{
			Fixture: expected.Fixture, Concepts: summary.Concepts,
			Errors: summary.Errors, Warnings: summary.Warnings,
			Findings: normalizeFindings(findings),
		}
		encoded, err := json.Marshal(actual)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(encoded, line) {
			t.Fatalf("normalized validation for %s\n got: %s\nwant: %s", expected.Fixture, encoded, line)
		}
	}
}

func normalizeFindings(findings []Finding) []normalizedFinding {
	result := make([]normalizedFinding, 0, len(findings))
	for _, finding := range findings {
		line, column := finding.Line, finding.Column
		if finding.Code == CodeInvalidYAML {
			line, column = nil, nil
		}
		result = append(result, normalizedFinding{
			Severity: finding.Severity, Code: finding.Code, Path: finding.Path,
			Line: line, Column: column, SpecSection: finding.SpecSection,
		})
	}
	return result
}

func TestValidateBundleBaseConformanceFixtures(t *testing.T) {
	tests := []struct {
		name     string
		concepts int
		errors   int
		warnings int
		codes    []FindingCode
	}{
		{name: "valid/minimal", concepts: 1},
		{name: "invalid/missing-frontmatter", concepts: 1, errors: 1, codes: []FindingCode{CodeMissingOpeningDelimiter}},
		{name: "invalid/missing-type", concepts: 1, errors: 1, codes: []FindingCode{CodeMissingType}},
		{name: "invalid/invalid-yaml", concepts: 1, errors: 1, codes: []FindingCode{CodeInvalidYAML}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join("..", "testdata", "okf", "v0.2", filepath.FromSlash(test.name))
			var findings []Finding
			summary, err := ValidateBundle(context.Background(), root, ValidateOptions{}, func(finding Finding) error {
				findings = append(findings, finding)
				return nil
			})
			if err != nil {
				t.Fatalf("ValidateBundle: %v", err)
			}
			if summary.TargetVersion != "0.2" || summary.Concepts != test.concepts || summary.Errors != test.errors || summary.Warnings != test.warnings {
				t.Fatalf("summary = %+v", summary)
			}
			if summary.OK() != (test.errors == 0) {
				t.Fatalf("summary.OK() = %v", summary.OK())
			}
			assertFindingCodes(t, findings, test.codes...)
		})
	}
}

func TestValidateBundleTypeMustBeNonEmptyString(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want bool
	}{
		{name: "string", yaml: "type: Reference\n", want: true},
		{name: "alias", yaml: "kind: &kind Reference\ntype: *kind\n", want: true},
		{name: "missing", yaml: "title: Missing\n"},
		{name: "number", yaml: "type: 42\n"},
		{name: "empty", yaml: "type: '   '\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document, parseFindings, err := ParseDocument("concept.md", []byte("---\n"+test.yaml+"---\n"))
			if err != nil || len(parseFindings) != 0 {
				t.Fatalf("ParseDocument = findings:%v error:%v", findingCodes(parseFindings), err)
			}
			findings := validateConceptBase(document)
			if test.want && len(findings) != 0 {
				t.Fatalf("valid type findings = %v", findingCodes(findings))
			}
			if !test.want {
				assertFindingCodes(t, findings, CodeMissingType)
				if test.name != "missing" && (findings[0].Line == nil || findings[0].Column == nil) {
					t.Fatal("type finding has no source position")
				}
			}
		})
	}
}

func TestValidateBundleEmitsFindingsInBytewisePathOrder(t *testing.T) {
	root := t.TempDir()
	writeConcept := func(path, source string) {
		t.Helper()
		fullPath := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConcept("a/z.md", "---\ntitle: Missing\n---\n")
	writeConcept("a-.md", "no frontmatter\n")
	writeConcept("b.md", "---\ntype: 42\n---\n")

	var paths []string
	summary, err := ValidateBundle(context.Background(), root, ValidateOptions{TargetVersion: "0.2"}, func(finding Finding) error {
		paths = append(paths, finding.Path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a-.md", "a/z.md", "b.md"}
	if !equalStrings(paths, want) {
		t.Fatalf("finding paths = %q, want %q", paths, want)
	}
	if summary.Concepts != 3 || summary.Errors != 3 {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestValidateBundleSinkErrorAndCancellation(t *testing.T) {
	root := filepath.Join("..", "testdata", "okf", "v0.2", "invalid", "missing-type")
	want := errors.New("stop")
	_, err := ValidateBundle(context.Background(), root, ValidateOptions{}, func(Finding) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("sink error = %v, want %v", err, want)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ValidateBundle(ctx, root, ValidateOptions{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled validation error = %v", err)
	}
}

func TestValidateBundleDoesNotFollowSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	bundle := t.TempDir()
	external := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(external, []byte("---\ntitle: would fail if followed\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(bundle, "linked.md")); err != nil {
		t.Fatal(err)
	}

	var findings []Finding
	summary, err := ValidateBundle(context.Background(), bundle, ValidateOptions{}, func(finding Finding) error {
		findings = append(findings, finding)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Concepts != 0 || summary.Errors != 0 || summary.Warnings != 1 {
		t.Fatalf("summary = %+v", summary)
	}
	assertFindingCodes(t, findings, CodeIgnoredSpecialFile)
	if findings[0].Path != "linked.md" {
		t.Fatalf("symlink finding path = %q", findings[0].Path)
	}
}

func TestValidationStageIsPrivateAndRemoved(t *testing.T) {
	stage, err := newValidationStage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	directory, path := stage.directory, stage.path
	if runtime.GOOS != "windows" {
		directoryInfo, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		fileInfo, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if directoryInfo.Mode().Perm() != 0o700 || fileInfo.Mode().Perm() != 0o600 {
			t.Fatalf("stage permissions = dir:%o file:%o", directoryInfo.Mode().Perm(), fileInfo.Mode().Perm())
		}
	}
	if err := stage.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("stage directory remains after close: %v", err)
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
