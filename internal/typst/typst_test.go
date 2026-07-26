package typst

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
)

func TestSourceEscapesAndPreservesFormatting(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nA *word* and **strong** text with \\#literal.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	source := Source(doc, style.Trade("en"))
	for _, want := range []string{"#emph[word]", "#strong[strong]", `\\#literal`} {
		if !strings.Contains(source, want) {
			t.Errorf("Typst source missing %q", want)
		}
	}
}

func TestSourceEscapesTypstReferencesAndLiteralAngleBrackets(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\n**1. Numbered bold lead.** Body text alpha follows here. Find @ColtonBruc3, \\<Moroni>, and a stray `. [^1]\n\n[^1]: A footnote with a stray `.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	source := Source(doc, style.Trade("en"))
	for _, want := range []string{`#strong[#h(0pt)1. Numbered bold lead.]`, `\@ColtonBruc3`, `\<Moroni\>`, "\\`"} {
		if !strings.Contains(source, want) {
			t.Errorf("Typst source missing escaped literal %q:\n%s", want, source)
		}
	}
}

func TestThematicBreakRendersAsOrnament(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nBefore.\n\n---\n\nAfter.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	if source := Source(doc, style.Trade("en")); !strings.Contains(source, "• • •") {
		t.Fatalf("Typst source is missing thematic-break ornament:\n%s", source)
	}
}

func TestNestedListsRenderWithIndentation(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("- Top level\n  - Nested child\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	if source := Source(doc, style.Trade("en")); !strings.Contains(source, "- Top level\n  - Nested child") {
		t.Fatalf("Typst source lost nested list indentation:\n%s", source)
	}
}

func TestPartEntryRendersAsPartOpener(t *testing.T) {
	doc := &markdown.Document{BookKind: "part", Title: "Part I: Beginnings"}
	source := Source(doc, style.Trade("en"))
	if !strings.Contains(source, "Part I: Beginnings") || !strings.Contains(source, "size: 22pt") {
		t.Fatalf("Typst source missing part opener:\n%s", source)
	}
}

func TestSourceDocumentsCreatesLinkedTOCWithFinalPageCounters(t *testing.T) {
	toc := &markdown.Document{BookID: "contents", BookKind: "toc", Title: "Contents", ExcludeFromTOC: true}
	part := &markdown.Document{BookID: "part-one", BookKind: "part", Title: "Part I: Beginnings"}
	chapter, issues := markdown.Parse([]byte("# First Chapter\n\nText.\n"))
	if len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	chapter.BookID, chapter.BookKind = "first-chapter", "chapter"
	source := SourceDocuments([]*markdown.Document{toc, part, chapter}, style.Trade("en"))
	for _, want := range []string{
		`#label("bookset-toc-part-one")`,
		`#label("bookset-toc-first-chapter")`,
		`#link(label("bookset-toc-part-one"))[Part I: Beginnings]`,
		`#context counter(page).at(label("bookset-toc-first-chapter")).first()`,
		`#heading(level: 1, outlined: false, bookmarked: true)[Part I: Beginnings]`,
		`#heading(level: 2, outlined: false, bookmarked: true)[First Chapter]`,
		`#h(1.25em)#link(label("bookset-toc-first-chapter"))[First Chapter]`,
	} {
		if !strings.Contains(source, want) {
			t.Errorf("Typst source missing %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, `#label("bookset-toc-contents")`) {
		t.Fatalf("TOC must not label or list itself:\n%s", source)
	}
}

func TestSourceDocumentsSetsFrontAndMainFolioPolicy(t *testing.T) {
	front := &markdown.Document{BookID: "contents", BookKind: "toc", PrintSection: "front", Title: "Contents", ExcludeFromTOC: true}
	chapter, issues := markdown.Parse([]byte("# Chapter One\n\nText.\n"))
	if len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	chapter.BookID, chapter.BookKind, chapter.PrintSection = "one", "chapter", "main"
	source := SourceDocuments([]*markdown.Document{front, chapter}, style.Trade("en"))
	for _, want := range []string{`#bookset-folios.update("roman")`, `#bookset-running-heads.update(false)`, `#counter(page).update(1)`} {
		if !strings.Contains(source, want) {
			t.Errorf("source missing %q:\n%s", want, source)
		}
	}
}

func TestSourceMarkersAndDiagnosticMapping(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nParagraph.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	doc.SourcePath = "chapters/example.md"
	source := Source(doc, style.Trade("en"))
	if !strings.Contains(source, "// bookset-source: chapters/example.md:3") {
		t.Fatalf("Typst source missing source marker:\n%s", source)
	}
	line := 0
	for index, value := range strings.Split(source, "\n") {
		if strings.Contains(value, "#par(") {
			line = index + 1
			break
		}
	}
	if line == 0 {
		t.Fatal("paragraph source line missing")
	}
	diagnostic := fmt.Sprintf("error: example\n  ┌─ book.typ:%d:3", line)
	if got := sourceLocationForTypstDiagnostic(source, diagnostic); got != "chapters/example.md:3" {
		t.Fatalf("mapped location=%q, want chapters/example.md:3", got)
	}
}

func TestRunningHeadsUseBookAndChapterTitles(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Chapter Heading\n\nText.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg := style.Trade("en")
	cfg.BookTitle = "The Book"
	source := Source(doc, cfg)
	for _, want := range []string{"The Book", "Chapter Heading", "numbering: none", "calc.even(p)"} {
		if !strings.Contains(source, want) {
			t.Errorf("running-head source missing %q", want)
		}
	}
}

func TestRunningHeadsTrackEachChapter(t *testing.T) {
	first, firstIssues := markdown.Parse([]byte("# First Chapter\n\nFirst text.\n"))
	second, secondIssues := markdown.Parse([]byte("# Second Chapter\n\nSecond text.\n"))
	if issues := append(firstIssues, secondIssues...); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg := style.Trade("en")
	source := SourceDocuments([]*markdown.Document{first, second}, cfg)
	if strings.Count(source, "#bookset-chapter.update") < 2 {
		t.Fatalf("chapter state updates missing from source:\n%s", source)
	}
	if !strings.Contains(source, "#bookset-chapter.update([Second Chapter])\n#pagebreak()") {
		t.Fatal("second chapter state must be updated before its page break")
	}
	if !strings.Contains(source, "First Chapter") || !strings.Contains(source, "Second Chapter") {
		t.Fatal("both chapter titles missing from source")
	}
}

func TestTimelinePaginationUsesNaturalFlowAndBreaksAfterTimeline(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\n## Timeline\n\n- **1:** One.\n- **2:** Two.\n- **3:** Three.\n- **4:** Four.\n- **5:** Five.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg, ok := style.Preset("timeline-trade", "en")
	if !ok {
		t.Fatal("timeline-trade preset unavailable")
	}
	source := Source(doc, cfg)
	if strings.Count(source, "#pagebreak()") != 1 {
		t.Fatalf("expected a post-timeline page break, source was:\n%s", source)
	}
	if !strings.Contains(source, "#timeline-item([5], [ Five.])\n#pagebreak()") {
		t.Fatalf("timeline page break missing after final item, source was:\n%s", source)
	}
}

func TestTimelineTradeSubheadingsAreUnnumbered(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\n## First Section\n\nText.\n\n## Second Section\n\nMore text.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg, ok := style.Preset("timeline-trade", "en")
	if !ok {
		t.Fatal("timeline-trade preset unavailable")
	}
	source := Source(doc, cfg)
	if strings.Contains(source, "= I. First Section") || strings.Contains(source, "= II. Second Section") {
		t.Fatalf("timeline-trade subheadings should not be numbered, source was:\n%s", source)
	}
	for _, want := range []string{"== First Section", "== Second Section"} {
		if !strings.Contains(source, want) {
			t.Errorf("source missing unnumbered heading %q", want)
		}
	}
}

func TestTemplateReceivesStyleTypography(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nText.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg := style.Trade("en")
	cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont = "Test Body", "Test Heading", "Test Utility"
	templateText, err := os.ReadFile(filepath.Join("..", "..", "templates", "timeline-trade", "chapter.typ"))
	if err != nil {
		t.Fatal(err)
	}
	source := sourceFromTemplate(doc, cfg, string(templateText))
	for _, want := range []string{`font: "Test Body"`, `font: "Test Heading"`, `font: "Test Utility"`} {
		if !strings.Contains(source, want) {
			t.Errorf("template output missing %q", want)
		}
	}
}

func TestTimelineTradeFootnotesAvoidOversizedLeading(t *testing.T) {
	templateText, err := os.ReadFile(filepath.Join("..", "..", "templates", "timeline-trade", "chapter.typ"))
	if err != nil {
		t.Fatal(err)
	}
	template := string(templateText)
	if !strings.Contains(template, "#show footnote.entry: set par(leading: 4pt)") {
		t.Fatal("timeline-trade footnotes should use the compact 4pt line-box gap")
	}
	for _, oversized := range []string{"set par(leading: 5.6pt)", "set par(leading: 7.4pt)"} {
		if strings.Contains(template, oversized) {
			t.Fatalf("timeline-trade footnotes regressed to oversized leading: %s", oversized)
		}
	}
}

func TestValidateFontsChecksAvailabilityAndChecksum(t *testing.T) {
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "body.ttf")
	fontData := []byte("synthetic font bytes")
	if err := os.WriteFile(fontPath, fontData, 0600); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(fontData)
	manifest := filepath.Join(dir, "fonts.toml")
	checksum := hex.EncodeToString(hash[:])
	manifestText := "[[font]]\nfamily = \"Test Body\"\npath = \"body.ttf\"\nsha256 = \"" + checksum + "\"\n\n[[font]]\nfamily = \"Test Heading\"\npath = \"body.ttf\"\nsha256 = \"" + checksum + "\"\n\n[[font]]\nfamily = \"Test Utility\"\npath = \"body.ttf\"\nsha256 = \"" + checksum + "\"\n"
	if err := os.WriteFile(manifest, []byte(manifestText), 0600); err != nil {
		t.Fatal(err)
	}
	typstPath := filepath.Join(dir, "typst")
	if err := os.WriteFile(typstPath, []byte("#!/bin/sh\nprintf '%s\\n' 'Test Body' 'Test Heading' 'Test Utility'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := style.Trade("en")
	cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont = "Test Body", "Test Heading", "Test Utility"
	cfg.FontManifest = manifest
	if err := validateFonts(typstPath, cfg); err != nil {
		t.Fatal(err)
	}
	incomplete := "[[font]]\nfamily = \"Test Body\"\npath = \"body.ttf\"\nsha256 = \"" + checksum + "\"\n"
	if err := os.WriteFile(manifest, []byte(incomplete), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateFonts(typstPath, cfg); err == nil || !strings.Contains(err.Error(), "does not lock required family") {
		t.Fatalf("expected incomplete manifest failure, got %v", err)
	}
	duplicate := incomplete + "\n[[font]]\nfamily = \"Test Body\"\npath = \"body.ttf\"\nsha256 = \"" + checksum + "\"\n"
	if err := os.WriteFile(manifest, []byte(duplicate), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateFonts(typstPath, cfg); err == nil || !strings.Contains(err.Error(), "duplicate family") {
		t.Fatalf("expected duplicate manifest failure, got %v", err)
	}
	if err := os.WriteFile(manifest, []byte(manifestText), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fontPath, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateFonts(typstPath, cfg); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum failure, got %v", err)
	}
}

func TestValidateConfiguredFontsFailsBeforeRendering(t *testing.T) {
	dir := t.TempDir()
	typstPath := filepath.Join(dir, "typst")
	if err := os.WriteFile(typstPath, []byte("#!/bin/sh\nprintf '%s\\n' 'Source Serif 4'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := style.Trade("en")
	cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont = "Source Serif 4", "Source Serif 4", "Source Sans 3"
	if err := validateConfiguredFonts(typstPath, cfg); err == nil || !strings.Contains(err.Error(), "Source Sans 3") {
		t.Fatalf("expected missing configured font failure, got %v", err)
	}
}

func TestValidateConfiguredFontsUsesConfiguredFontDir(t *testing.T) {
	dir := t.TempDir()
	fontDir := filepath.Join(dir, "vendor", "fonts")
	if err := os.MkdirAll(fontDir, 0755); err != nil {
		t.Fatal(err)
	}
	typstPath := filepath.Join(dir, "typst")
	script := "#!/bin/sh\nif [ \"$1\" != \"fonts\" ] || [ \"$2\" != \"--font-path\" ] || [ \"$3\" != \"" + fontDir + "\" ]; then\n  exit 1\nfi\nprintf '%s\\n' 'Vendored Serif' 'Vendored Sans'\n"
	if err := os.WriteFile(typstPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := style.Trade("en")
	cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont = "Vendored Serif", "Vendored Serif", "Vendored Sans"
	cfg.FontDir = fontDir
	if err := validateConfiguredFonts(typstPath, cfg); err != nil {
		t.Fatalf("vendored fonts were not checked with their configured font path: %v", err)
	}
}

func TestPDFSmokeAndDeterminismWhenTypstAvailable(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not installed")
	}
	if err := CheckConfiguredFonts(style.Trade("en")); err != nil {
		t.Skip("configured PDF test fonts unavailable: " + err.Error())
	}
	doc, parseIssues := markdown.Parse([]byte("# Title\n\n**1. Numbered bold lead.** Body text alpha follows here. A *word* from @ColtonBruc3, a transcription \\<Moroni>, and a stray `. [^1]\n\n- Parent item\n  - Nested item\n\n---\n\n**Strong** text.\n\n[^1]: Footnote with a stray `.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	dir := t.TempDir()
	first, second := filepath.Join(dir, "one.pdf"), filepath.Join(dir, "two.pdf")
	if err := Render(first, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	if err := Render(second, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	if pdftotext, err := exec.LookPath("pdftotext"); err == nil {
		text, extractErr := exec.Command(pdftotext, first, "-").Output()
		if extractErr != nil {
			t.Fatal(extractErr)
		}
		for _, want := range []string{"1. Numbered bold lead.", "Body text alpha follows here."} {
			if !strings.Contains(string(text), want) {
				t.Fatalf("PDF lost numbered bold lead text %q:\n%s", want, text)
			}
		}
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Fatal("PDF output is not deterministic")
	}
}

func TestRenderWithOptionsWritesGeneratedSource(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not installed")
	}
	if err := CheckConfiguredFonts(style.Trade("en")); err != nil {
		t.Skip("configured PDF test fonts unavailable: " + err.Error())
	}
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nText.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "book.typ")
	if err := RenderWithOptions(filepath.Join(dir, "book.pdf"), doc, style.Trade("en"), RenderOptions{SourcePath: sourcePath}); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(source), "= Title") {
		t.Fatalf("generated Typst source was not written: %s", source)
	}
}
