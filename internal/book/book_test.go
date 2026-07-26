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
}
