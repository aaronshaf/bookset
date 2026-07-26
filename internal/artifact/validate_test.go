package artifact

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aaronshaf/bookset/internal/epub"
	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
	"github.com/aaronshaf/bookset/internal/typst"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func fixture(t *testing.T) *markdown.Document {
	t.Helper()
	source := []byte("# Title\n\nA *word* and **strong** text.[^1]\n\n[^1]: A note.\n")
	doc, parseIssues := markdown.Parse(source)
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	return doc
}

func TestEPUBArtifactValidation(t *testing.T) {
	doc := fixture(t)
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := epub.Write(path, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	if issues := Validate(path, doc); len(issues) != 0 {
		t.Fatal(SortedMessages(issues))
	}
}

func TestInspectEPUBProducesStableReport(t *testing.T) {
	doc := fixture(t)
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := epub.Write(path, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	report, err := InspectArtifact(path)
	if err != nil {
		t.Fatal(err)
	}
	if report.Schema != "bookset.artifact-inspection.v1" || report.Status != "ok" {
		t.Fatalf("unexpected report header: %#v", report)
	}
	if report.Artifact.Format != "epub" || report.Artifact.SHA256 == "" || len(report.Artifact.EPUB.ChapterFiles) != 1 {
		t.Fatalf("incomplete EPUB report: %#v", report.Artifact)
	}
	if len(report.Checks) != 2 || report.Checks[0].Code != "epub.structure" || report.Checks[1].Code != "epub.chapters" {
		t.Fatalf("unexpected checks: %#v", report.Checks)
	}
}

func TestInspectionReportMatchesVersionedContract(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "docs", "schemas", "artifact-inspection.v1.json")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument map[string]any
	if err := json.Unmarshal(schema, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	const schemaID = "https://github.com/aaronshaf/bookset/schema/artifact-inspection.v1.json"
	if schemaDocument["$id"] != schemaID {
		t.Fatalf("unexpected schema id: %v", schemaDocument["$id"])
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(schemaID, schemaDocument); err != nil {
		t.Fatal(err)
	}
	contract, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatal(err)
	}
	doc := fixture(t)
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := epub.Write(path, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	report, err := InspectArtifactAgainst(path, []*markdown.Document{doc})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "status", "artifact", "checks", "issues"} {
		if _, ok := payload[key]; !ok {
			t.Errorf("inspection report missing required field %q", key)
		}
	}
	if payload["schema"] != "bookset.artifact-inspection.v1" {
		t.Fatalf("unexpected report schema: %v", payload["schema"])
	}
	if err := contract.Validate(payload); err != nil {
		t.Fatalf("valid report rejected by schema: %v", err)
	}
	payload["status"] = "published"
	if err := contract.Validate(payload); err == nil {
		t.Fatal("schema accepted an invalid status")
	}
}

func TestInspectArtifactAgainstAddsFidelityIssues(t *testing.T) {
	doc := fixture(t)
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := epub.Write(path, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	report, err := InspectArtifactAgainst(path, []*markdown.Document{doc})
	if err != nil || report.Status != "ok" || len(report.Issues) != 0 {
		t.Fatalf("valid artifact reported issues: err=%v report=%#v", err, report)
	}
	wrong, parseIssues := markdown.Parse([]byte("# Different title\n\nUnrelated text.\n"))
	if issues := markdown.Validate(wrong, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	report, err = InspectArtifactAgainst(path, []*markdown.Document{wrong})
	if err != nil || report.Status != "error" || len(report.Issues) == 0 || report.Issues[0].Code != "fidelity.text" {
		t.Fatalf("missing fidelity failure: err=%v report=%#v", err, report)
	}
}

func TestMissingFragmentNormalizesTrackedHeadingText(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\n## The Biblical Pattern\n\nText.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	if missing := missingFragment("Title T H E B I B L I C A L PAT T E R N Text.", []*markdown.Document{doc}); missing != "" {
		t.Fatalf("tracked heading was reported missing: %q", missing)
	}
}

func TestPDFArtifactValidationWhenToolsAvailable(t *testing.T) {
	for _, name := range []string{"typst", "pdftotext", "pdfinfo", "pdffonts"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " is not installed")
		}
	}
	doc := fixture(t)
	path := filepath.Join(t.TempDir(), "book.pdf")
	if err := typst.Render(path, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	if issues := Validate(path, doc); len(issues) != 0 {
		t.Fatal(SortedMessages(issues))
	}
}
