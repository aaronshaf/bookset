package typst

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

func TestPDFSmokeAndDeterminismWhenTypstAvailable(t *testing.T) {
	if _, err := exec.LookPath("typst"); err != nil {
		t.Skip("typst not installed")
	}
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nA *word* and **strong** text.\n"))
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
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Fatal("PDF output is not deterministic")
	}
}
