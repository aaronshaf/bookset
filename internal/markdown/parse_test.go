package markdown

import (
	"os"
	"strings"
	"testing"
)

func TestFieldNotesPreserveModel(t *testing.T) {
	source, err := os.ReadFile("../../testdata/field-notes.md")
	if err != nil {
		t.Fatal(err)
	}
	doc, parseIssues := Parse(source)
	if issues := Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(FormatIssues(issues))
	}
	if len(doc.Blocks) != 8 || len(doc.Footnotes) != 1 {
		t.Fatalf("blocks=%d footnotes=%d", len(doc.Blocks), len(doc.Footnotes))
	}
	plain := doc.PlainText()
	for _, want := range []string{"Field Notes on Coastal Observation", "useful distinction", "measured observation", "regular intervals."} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain text missing %q", want)
		}
	}
}

func TestNestedFormattingAndEscapes(t *testing.T) {
	doc, parseIssues := Parse([]byte("# Title\n\nA *slanted **strong** word* and a literal \\#tag.\n"))
	if issues := Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(FormatIssues(issues))
	}
	if got := doc.PlainText(); !strings.Contains(got, "slanted strong word") || !strings.Contains(got, "#tag") {
		t.Fatalf("plain text=%q", got)
	}
	paragraph := doc.Blocks[1]
	if paragraph.Inlines[1].Kind != Emphasis || paragraph.Inlines[1].Children[1].Kind != Strong {
		t.Fatalf("nested formatting was not preserved: %#v", paragraph.Inlines)
	}
}

func TestFootnoteValidation(t *testing.T) {
	doc, parseIssues := Parse([]byte("Text[^missing].\n\n[^unused]: not used\n"))
	issues := Validate(doc, parseIssues)
	got := FormatIssues(issues)
	if !strings.Contains(got, "undefined footnote") || !strings.Contains(got, "unused footnote") {
		t.Fatalf("issues=%q", got)
	}
}

func TestLiteralAngleBracketsAndEntitiesPreserveText(t *testing.T) {
	source := []byte("# Title\n\nA transcription reads \\<Moroni> and &lt;Nephi&gt;.\n")
	doc, parseIssues := Parse(source)
	if issues := Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(FormatIssues(issues))
	}
	if got := doc.PlainText(); strings.Contains(got, `\<Moroni>`) || !strings.Contains(got, "<Moroni>") || !strings.Contains(got, "<Nephi>") {
		t.Fatalf("literal angle-bracket text was lost or not decoded: %q", got)
	}
}

func TestThematicBreakIsSupported(t *testing.T) {
	doc, parseIssues := Parse([]byte("# Title\n\nBefore.\n\n---\n\nAfter.\n"))
	if issues := Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(FormatIssues(issues))
	}
	if len(doc.Blocks) != 4 || doc.Blocks[2].Kind != ThematicBreak {
		t.Fatalf("thematic break was not preserved: %#v", doc.Blocks)
	}
}
