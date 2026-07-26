package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/aaronshaf/bookset/internal/artifact"
	"github.com/aaronshaf/bookset/internal/epub"
	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/semantic"
	"github.com/aaronshaf/bookset/internal/style"
	"github.com/aaronshaf/bookset/internal/typst"
)

func TestSemanticChapterPublishesToBothFormats(t *testing.T) {
	source, err := os.ReadFile(filepath.Join("..", "..", "testdata", "semantic-chapter.md"))
	if err != nil {
		t.Fatal(err)
	}
	doc, parseIssues := markdown.Parse(source)
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg, err := style.LoadFile(filepath.Join("..", "..", "styles", "timeline-trade.toml"), doc.Language)
	if err != nil {
		t.Fatal(err)
	}
	normalized := semantic.Normalize(doc, cfg)
	if len(normalized.Blocks) < 5 || normalized.Blocks[0].Kind != semantic.ChapterOpener || normalized.Blocks[1].Kind != semantic.ThenNow || normalized.Blocks[3].Kind != semantic.Timeline {
		t.Fatalf("unexpected semantic chapter: %#v", normalized.Blocks)
	}

	dir := t.TempDir()
	epubOne, epubTwo := filepath.Join(dir, "one.epub"), filepath.Join(dir, "two.epub")
	if err := epub.Write(epubOne, doc, cfg); err != nil {
		t.Fatal(err)
	}
	if err := epub.Write(epubTwo, doc, cfg); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.Validate(epubOne, doc); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	first, err := os.ReadFile(epubOne)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(epubTwo)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("semantic EPUB output is not deterministic")
	}

	for _, tool := range []string{"typst", "pdftotext", "pdfinfo", "pdffonts"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip(tool + " is not installed; EPUB coverage completed")
		}
	}
	if err := typst.CheckConfiguredFonts(cfg); err != nil {
		t.Skip("configured PDF test fonts unavailable; EPUB coverage completed: " + err.Error())
	}
	pdfOne, pdfTwo := filepath.Join(dir, "one.pdf"), filepath.Join(dir, "two.pdf")
	if err := typst.Render(pdfOne, doc, cfg); err != nil {
		t.Fatal(err)
	}
	if err := typst.Render(pdfTwo, doc, cfg); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.Validate(pdfOne, doc); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	first, err = os.ReadFile(pdfOne)
	if err != nil {
		t.Fatal(err)
	}
	second, err = os.ReadFile(pdfTwo)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("semantic PDF output is not deterministic")
	}
}
