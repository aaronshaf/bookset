package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadResolvesTemplateDirectoryRelativeToProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bookset.toml")
	if err := os.WriteFile(path, []byte("[book]\ntitle = \"Test\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	project, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if project.BaseDir != dir {
		t.Fatalf("base dir=%q, want %q", project.BaseDir, dir)
	}
	if project.Templates.Dir != filepath.Join(dir, "templates") {
		t.Fatalf("templates=%q", project.Templates.Dir)
	}
}
