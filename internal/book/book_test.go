package book

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aaronshaf/bookset/internal/config"
)

func TestLoadBookManifest(t *testing.T) {
	path := filepath.Join("..", "..", "bookset.book.example.toml")
	project, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	manuscript, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(manuscript.Chapters) != 2 {
		t.Fatalf("chapters=%d", len(manuscript.Chapters))
	}
	if manuscript.Style.Name != "timeline-trade" {
		t.Fatalf("style=%q", manuscript.Style.Name)
	}
	for index, want := range []string{"CHAPTER 1", "CHAPTER 2"} {
		if got := manuscript.Chapters[index].ChapterLabel; got != want {
			t.Errorf("chapter %d label=%q, want %q", index+1, got, want)
		}
	}
}

func TestLoadTypedBookManifest(t *testing.T) {
	project, err := config.Load(filepath.Join("..", "..", "bookset.contents.example.toml"))
	if err != nil {
		t.Fatal(err)
	}
	manuscript, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(manuscript.Documents) != 4 || manuscript.Documents[0].BookKind != "toc" || len(manuscript.Chapters) != 2 {
		t.Fatalf("unexpected typed manuscript: %#v", manuscript)
	}
}

func TestLoadTypedContentsPreservesOrderAndChapterNumbers(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	front := write("front.md", "# Preface\n\nOpening material.\n")
	first := write("first.md", "# First Chapter\n\nFirst body.\n")
	second := write("second.md", "# Second Chapter\n\nSecond body.\n")
	back := write("back.md", "# Notes\n\nBack matter.\n")
	project := config.Project{Book: config.Book{Language: "en", ChapterLabel: "CHAPTER", ChapterNumbering: true}, Contents: []config.Content{
		{ID: "preface", Kind: "front-matter", Source: front},
		{ID: "contents", Kind: "toc"},
		{ID: "part-one", Kind: "part", Title: "Part I: Beginnings"},
		{ID: "first", Kind: "chapter", Source: first},
		{ID: "second", Kind: "chapter", Source: second},
		{ID: "notes", Kind: "back-matter", Source: back},
	}}
	manuscript, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if len(manuscript.Documents) != 6 || len(manuscript.Chapters) != 2 {
		t.Fatalf("documents=%d chapters=%d", len(manuscript.Documents), len(manuscript.Chapters))
	}
	if got := manuscript.Documents[2].BookKind; got != "part" {
		t.Fatalf("part kind=%q", got)
	}
	for index, want := range []string{"CHAPTER 1", "CHAPTER 2"} {
		if got := manuscript.Chapters[index].ChapterLabel; got != want {
			t.Errorf("chapter %d label=%q, want %q", index+1, got, want)
		}
	}
	if !manuscript.Documents[0].ExcludeFromTOC || !manuscript.Documents[1].ExcludeFromTOC || manuscript.Documents[2].ExcludeFromTOC || manuscript.Documents[3].ExcludeFromTOC || manuscript.Documents[4].ExcludeFromTOC || !manuscript.Documents[5].ExcludeFromTOC {
		t.Fatalf("unexpected TOC defaults: %#v", manuscript.Documents)
	}
}

func TestLoadTOCEntryDefaultsTitleAndRejectsInvalidForms(t *testing.T) {
	project := config.Project{Contents: []config.Content{{ID: "contents", Kind: "toc", Style: "trade"}, {ID: "part", Kind: "part", Title: "Part I", Style: "trade"}}}
	manuscript, err := Load(project)
	if err != nil {
		t.Fatal(err)
	}
	if got := manuscript.Documents[0].Title; got != "Contents" {
		t.Fatalf("TOC title=%q, want Contents", got)
	}
	if !manuscript.Documents[0].ExcludeFromTOC {
		t.Fatal("synthetic TOC must not include itself")
	}
	for _, contents := range [][]config.Content{
		{{ID: "contents", Kind: "toc", Source: "contents.md", Style: "trade"}},
		{{ID: "first", Kind: "toc", Style: "trade"}, {ID: "second", Kind: "toc", Style: "trade"}},
	} {
		if _, err := Load(config.Project{Contents: contents}); err == nil {
			t.Fatalf("invalid TOC contents were accepted: %#v", contents)
		}
	}
}

func TestLoadRejectsMixedLegacyAndTypedManifest(t *testing.T) {
	_, err := Load(config.Project{Chapters: []config.Chapter{{Source: "one.md"}}, Contents: []config.Content{{ID: "one", Kind: "chapter", Source: "one.md"}}})
	if err == nil {
		t.Fatal("mixed manifest was accepted")
	}
}

func TestResolvedChapterLabel(t *testing.T) {
	tests := []struct {
		override, defaultLabel string
		numbered               bool
		number                 int
		want                   string
	}{
		{defaultLabel: "CHAPTER", numbered: true, number: 40, want: "CHAPTER 40"},
		{defaultLabel: "CHAPTER", number: 40, want: "CHAPTER"},
		{override: "INTERLUDE", defaultLabel: "CHAPTER", numbered: true, number: 40, want: "INTERLUDE"},
		{numbered: true, number: 40, want: ""},
	}
	for _, test := range tests {
		if got := resolvedChapterLabel(test.override, test.defaultLabel, test.numbered, test.number); got != test.want {
			t.Errorf("resolvedChapterLabel(%q, %q, %t, %d) = %q, want %q", test.override, test.defaultLabel, test.numbered, test.number, got, test.want)
		}
	}
}
