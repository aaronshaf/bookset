package integration

import (
	"archive/zip"
	"io"
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
	if err := epub.WriteBook(epubPath, manuscript.Documents, manuscript.Style); err != nil {
		t.Fatal(err)
	}
	if err := epub.Validate(epubPath); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.ValidateDocuments(epubPath, manuscript.Documents); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	reader, err := zip.OpenReader(epubPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	seen := map[string]bool{}
	content := map[string]string{}
	for _, file := range reader.File {
		seen[file.Name] = true
		if strings.HasPrefix(file.Name, "OEBPS/content-") && strings.HasSuffix(file.Name, ".xhtml") {
			entry, openErr := file.Open()
			if openErr != nil {
				t.Fatal(openErr)
			}
			bytes, readErr := io.ReadAll(entry)
			_ = entry.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			content[file.Name] = string(bytes)
		}
	}
	for chapter, want := range map[string]string{"OEBPS/content-001.xhtml": "CHAPTER 1", "OEBPS/content-002.xhtml": "CHAPTER 2"} {
		if !strings.Contains(content[chapter], want) {
			t.Errorf("%s missing chapter label %q", chapter, want)
		}
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
	if err := typst.CheckConfiguredFonts(manuscript.Style); err != nil {
		t.Skip("configured PDF test fonts unavailable; EPUB book build completed: " + err.Error())
	}
	pdfPath := filepath.Join(dir, "book.pdf")
	if err := typst.RenderDocuments(pdfPath, manuscript.Documents, manuscript.Style); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.ValidateDocumentsWithStyle(pdfPath, manuscript.Documents, manuscript.Style); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	text, err := exec.Command("pdftotext", pdfPath, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "CHAPTER 2") {
		t.Fatalf("second chapter label was not emitted: %s", text)
	}
}

func TestTypedBookEPUBIncludesPartAndNavigation(t *testing.T) {
	project, err := config.Load(filepath.Join("..", "..", "bookset.contents.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	manuscript, err := book.Load(project)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "book.epub")
	if err := epub.WriteBook(path, manuscript.Documents, manuscript.Style); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var nav string
	contentCount := 0
	for _, file := range reader.File {
		if strings.HasPrefix(file.Name, "OEBPS/content-") && strings.HasSuffix(file.Name, ".xhtml") {
			contentCount++
		}
		if file.Name != "OEBPS/nav.xhtml" {
			continue
		}
		entry, openErr := file.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		data, readErr := io.ReadAll(entry)
		_ = entry.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		nav = string(data)
	}
	for _, want := range []string{"Part I: Beginnings", "The Practice of Careful History", "A Practical Method"} {
		if !strings.Contains(nav, want) {
			t.Errorf("EPUB navigation missing %q: %s", want, nav)
		}
	}
	if contentCount != 3 {
		t.Fatalf("EPUB spine contains %d documents, want 3 without synthetic PDF TOC", contentCount)
	}

	for _, tool := range []string{"typst", "pdftotext", "pdfinfo", "pdffonts"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skip("PDF rendering tools not installed; EPUB typed-book build completed")
		}
	}
	if err := typst.CheckConfiguredFonts(manuscript.Style); err != nil {
		t.Skip("configured PDF test fonts unavailable; EPUB typed-book build completed: " + err.Error())
	}
	pdfPath := filepath.Join(t.TempDir(), "book.pdf")
	if err := typst.RenderDocuments(pdfPath, manuscript.Documents, manuscript.Style); err != nil {
		t.Fatal(err)
	}
	if issues := artifact.ValidateDocumentsWithStyle(pdfPath, manuscript.Documents, manuscript.Style); len(issues) != 0 {
		t.Fatal(artifact.SortedMessages(issues))
	}
	text, err := exec.Command("pdftotext", pdfPath, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Contents", "Part I: Beginnings", "A Practical Method"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("PDF contents build missing %q: %s", want, text)
		}
	}
	proofPath := filepath.Join(t.TempDir(), "proof.pdf")
	proof, err := typst.ProofDocuments(proofPath, manuscript.Documents, manuscript.Style)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof) != 4 || proof[0].ID != "contents" || proof[0].StartPage != 3 || proof[1].Folio != 1 || proof[2].EndPage < proof[2].StartPage {
		t.Fatalf("unexpected final-layout proof: %#v", proof)
	}
}
