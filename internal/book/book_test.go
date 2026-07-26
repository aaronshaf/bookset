package book

import (
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
