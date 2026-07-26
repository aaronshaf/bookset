package epub

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
)

func TestDeterministicEPUB(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("---\ntitle: Test\nauthor: Author\nlanguage: en\n---\n\n# Title\n\nText[^1].\n\n[^1]: Note.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	dir := t.TempDir()
	first, second := filepath.Join(dir, "one.epub"), filepath.Join(dir, "two.epub")
	if err := Write(first, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	if err := Write(second, doc, style.Trade("en")); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(first)
	b, _ := os.ReadFile(second)
	if !bytes.Equal(a, b) {
		t.Fatal("EPUB output is not deterministic")
	}
	if err := Validate(first); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(first)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, file := range reader.File {
		if file.Name == "mimetype" && (file.Method != zip.Store || len(file.Extra) != 0) {
			t.Fatalf("mimetype must be stored without ZIP extra fields: method=%d extra=%d", file.Method, len(file.Extra))
		}
	}
}

func TestEPUBIdentifierIsStableAndContentDerived(t *testing.T) {
	first, firstIssues := markdown.Parse([]byte("---\ntitle: Test\nauthor: Author\nlanguage: en\n---\n\n# Title\n\nFirst text.\n"))
	second, secondIssues := markdown.Parse([]byte("---\ntitle: Test\nauthor: Author\nlanguage: en\n---\n\n# Title\n\nSecond text.\n"))
	if issues := markdown.Validate(first, firstIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	if issues := markdown.Validate(second, secondIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	if got, want := identifier([]*markdown.Document{first}), identifier([]*markdown.Document{first}); got != want {
		t.Fatalf("identifier is not stable: %q != %q", got, want)
	}
	if identifier([]*markdown.Document{first}) == identifier([]*markdown.Document{second}) {
		t.Fatal("identifier should change when book content changes")
	}
}

func TestSemanticStructuresReachEPUB(t *testing.T) {
	source := []byte("# Chapter\n\n**Then:** Earlier.\n\n**Now:** Today.\n\n## Timeline\n\n- **1840:** First.\n")
	doc, parseIssues := markdown.Parse(source)
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg, ok := style.Preset("timeline-trade", "en")
	if !ok {
		t.Fatal("preset unavailable")
	}
	cfg.ChapterLabel = "CHAPTER"
	content := content(doc, cfg)
	for _, want := range []string{`class="chapter-label"`, `class="then-now"`, `class="timeline"`, `class="timeline-item"`} {
		if !strings.Contains(content, want) {
			t.Errorf("EPUB content missing semantic marker %q", want)
		}
	}
}

func TestThematicBreakAndLiteralAngleBracketsReachEPUB(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Title\n\nA \\<Moroni>.\n\n---\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	content := content(doc, style.Trade("en"))
	for _, want := range []string{"&lt;Moroni&gt;", "<hr/>"} {
		if !strings.Contains(content, want) {
			t.Errorf("EPUB content missing %q: %s", want, content)
		}
	}
}

func TestNestedListsReachEPUB(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("- Top level\n  - Nested child\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	if content := content(doc, style.Trade("en")); !strings.Contains(content, "<li>Top level<ul><li>Nested child</li></ul></li>") {
		t.Fatalf("EPUB content lost nested list: %s", content)
	}
}

func TestBookNavigationHonorsTOCEligibility(t *testing.T) {
	front := &markdown.Document{Title: "Preface", ExcludeFromTOC: true}
	part := &markdown.Document{Title: "Part I"}
	chapter := &markdown.Document{Title: "Chapter One"}
	nav := bookNav([]*markdown.Document{front, part, chapter}, []string{"front.xhtml", "part.xhtml", "chapter.xhtml"})
	if strings.Contains(nav, "Preface") || !strings.Contains(nav, "Part I") || !strings.Contains(nav, "Chapter One") {
		t.Fatalf("unexpected EPUB navigation: %s", nav)
	}
}

func TestPrintTOCIsExcludedFromEPUBSpine(t *testing.T) {
	toc := &markdown.Document{BookKind: "toc", Title: "Contents", ExcludeFromTOC: true}
	chapter := &markdown.Document{BookKind: "chapter", Title: "Chapter One"}
	spine := spineDocuments([]*markdown.Document{toc, chapter})
	if len(spine) != 1 || spine[0] != chapter {
		t.Fatalf("unexpected EPUB spine: %#v", spine)
	}
	if nav := bookNav(spine, []string{"chapter.xhtml"}); strings.Contains(nav, `href="contents.xhtml"`) || !strings.Contains(nav, "Chapter One") {
		t.Fatalf("unexpected EPUB navigation: %s", nav)
	}
}

func TestBookNavigationNestsChaptersUnderParts(t *testing.T) {
	part := &markdown.Document{BookKind: "part", Title: "Part I"}
	chapter := &markdown.Document{BookKind: "chapter", Title: "Chapter One"}
	nav := bookNav([]*markdown.Document{part, chapter}, []string{"part.xhtml", "chapter.xhtml"})
	want := `<li><a href="part.xhtml">Part I</a><ol><li><a href="chapter.xhtml">Chapter One</a></li></ol></li>`
	if !strings.Contains(nav, want) {
		t.Fatalf("EPUB navigation is not nested: %s", nav)
	}
}

func TestBookNavigationIncludesSectionLandmarks(t *testing.T) {
	front := &markdown.Document{Title: "Preface", PrintSection: "front"}
	chapter := &markdown.Document{Title: "Chapter One", PrintSection: "main"}
	back := &markdown.Document{Title: "Notes", PrintSection: "back"}
	nav := bookNav([]*markdown.Document{front, chapter, back}, []string{"front.xhtml", "chapter.xhtml", "back.xhtml"})
	for _, want := range []string{`epub:type="landmarks"`, `epub:type="frontmatter" href="front.xhtml"`, `epub:type="bodymatter" href="chapter.xhtml"`, `epub:type="backmatter" href="back.xhtml"`} {
		if !strings.Contains(nav, want) {
			t.Errorf("navigation missing %q: %s", want, nav)
		}
	}
}

func TestBookOPFUsesBookMetadataAndModifiedTime(t *testing.T) {
	doc := &markdown.Document{Title: "Chapter", Author: "Chapter Author", Language: "en"}
	cfg := style.Trade("en")
	cfg.BookTitle, cfg.BookAuthor, cfg.BookModified = "Book Title", "Book Author", "2026-07-26T12:00:00Z"
	opf := bookOPF([]*markdown.Document{doc}, []string{"content.xhtml"}, cfg)
	for _, want := range []string{`<dc:title>Book Title</dc:title>`, `<dc:creator>Book Author</dc:creator>`, `<meta property="dcterms:modified">2026-07-26T12:00:00Z</meta>`, `property="schema:accessMode">textual`, `property="schema:accessibilityFeature">structuralNavigation`} {
		if !strings.Contains(opf, want) {
			t.Errorf("OPF missing %q: %s", want, opf)
		}
	}
}

func TestEPUBContentUsesReadingOrderAndFootnoteSemantics(t *testing.T) {
	doc, issues := markdown.Parse([]byte("# Preface\n\nText[^1].\n\n[^1]: Note.\n"))
	if issues := markdown.Validate(doc, issues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	doc.PrintSection = "front"
	rendered := content(doc, style.Trade("en"))
	for _, want := range []string{`xmlns:epub=`, `xml:lang="en"`, `body epub:type="frontmatter"`, `epub:type="noteref"`, `epub:type="footnotes"`, `epub:type="footnote"`} {
		if !strings.Contains(rendered, want) {
			t.Errorf("content missing %q: %s", want, rendered)
		}
	}
}

func TestStylesheetReceivesStyleTypography(t *testing.T) {
	cfg := style.Trade("en")
	cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont = "Test Body", "Test Heading", "Test Utility"
	rendered, err := renderStyles(css, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`font-family:"Test Body"`, `font-family:"Test Heading"`, `font-family:"Test Utility"`} {
		if !strings.Contains(string(rendered), want) {
			t.Errorf("stylesheet output missing %q", want)
		}
	}
}
