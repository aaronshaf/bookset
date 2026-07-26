package style

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aaronshaf/bookset/internal/config"
)

func TestApplyProjectOverridesPreset(t *testing.T) {
	cfg, ok := Preset("trade", "en")
	if !ok {
		t.Fatal("trade preset missing")
	}
	cfg, err := ApplyProject(cfg, config.Project{
		Book:       config.Book{Trim: "6x9"},
		Typography: config.Typography{BodyFont: "Test Serif", BodySize: "11pt", Leading: "5pt"},
		Layout:     config.Layout{InsideMargin: "1in", OutsideMargin: "0.8in", TopMargin: "0.7in", BottomMargin: "0.75in"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BodyFont != "Test Serif" || cfg.BodySize != "11pt" || cfg.Margin != "(inside: 1in, outside: 0.8in, top: 0.7in, bottom: 0.75in)" {
		t.Fatalf("project overrides not applied: %#v", cfg)
	}
}

func TestLoadFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.toml")
	if err := os.WriteFile(path, []byte("name = \"trade\"\n[typography]\nbody_font = \"Custom Serif\"\n[templates]\ndir = \"custom-templates\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFile(path, "en")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BodyFont != "Custom Serif" || cfg.TemplateDir != filepath.Join(filepath.Dir(path), "custom-templates") {
		t.Fatalf("file style not loaded: %#v", cfg)
	}
}

func TestRejectUnsupportedTrim(t *testing.T) {
	cfg, _ := Preset("trade", "en")
	if _, err := ApplyProject(cfg, config.Project{Book: config.Book{Trim: "5x8"}}); err == nil {
		t.Fatal("expected unsupported trim error")
	}
}
