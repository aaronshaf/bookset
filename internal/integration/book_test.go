package integration

import (
	"archive/zip"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaronshaf/bookset/internal/artifact"
	"github.com/aaronshaf/bookset/internal/book"
	"github.com/aaronshaf/bookset/internal/config"
	"github.com/aaronshaf/bookset/internal/epub"
	"github.com/aaronshaf/bookset/internal/typst"
)

func TestBookBuildProducesOrderedChapters(t *testing.T) {
	project, err := config.Load(filepath.Join("..", "..", "bookset.book.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	manuscript, err := book.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	epubPath := filepath.Join(dir, "book.epub")
	if err := epub.WriteBook(epubPath, manuscript.Chapters, manuscript.Style); err != nil {
		t.Fatal(err)
	}
	if err := epub.Validate(epubPath); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.ValidateDocuments(epubPath, manuscript.Chapters); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	reader, err := zip.OpenReader(epubPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	seen := map[string]bool{}
	for _, file := range reader.File {
		seen[file.Name] = true
	}
	for _, name := range []string{"OEBPS/content-001.xhtml", "OEBPS/content-002.xhtml", "OEBPS/nav.xhtml", "OEBPS/package.opf"} {
		if !seen[name] {
			t.Errorf("book EPUB missing %s", name)
		}
	}

	for _, tool := range []string{"typst", "pdftotext", "pdfinfo", "pdffonts"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("PDF rendering tools not installed; EPUB book build completed")
		}
	}
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := typst.RenderDocuments(pdfPath, manuscript.Chapters, manuscript.Style); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.ValidateDocuments(pdfPath, manuscript.Chapters); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	text, err := exec.Command("pdftotext", pdfPath, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(text), "The Practice of Careful History") < 2 {
		t.Fatalf("second chapter running head was not emitted: %s", text)
	}
}
