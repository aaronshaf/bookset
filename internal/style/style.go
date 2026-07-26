package style

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aaronshaf/bookset/internal/config"
	"github.com/pelletier/go-toml/v2"
)

// Config contains versionable typography/layout values and semantic labels
// consumed by the selected renderer template.
type Config struct {
	Name                   string
	Margin                 string
	BodyFont               string
	HeadingFont            string
	UtilityFont            string
	Language               string
	BodySize               string
	Leading                string
	Inside                 string
	Outside                string
	Top                    string
	Bottom                 string
	Trim                   string
	RunningHeads           bool
	FrontMatterFolios      string
	TemplateDir            string
	TemplateRequired       bool
	FontManifest           string
	FontDir                string
	ChapterLabel           string
	BookTitle              string
	BookAuthor             string
	BookModified           string
	PageBreakAfterThenNow  bool
	PageBreakAfterTimeline bool
	HideTimelineHeading    bool
	TimelinePageCount      int
	SectionNumbering       bool
	Sheet                  string
	TrimMarks              bool
}

func Preset(name, language string) (Config, bool) {
	if language == "" {
		language = "en"
	}
	switch name {
	case "trade":
		return Config{Name: name, Margin: "0.78in", BodyFont: "Source Serif 4", HeadingFont: "Source Serif 4", UtilityFont: "Source Sans 3", Language: language, BodySize: "10.25pt", Leading: "14.5pt", Trim: "6x9", RunningHeads: true, FrontMatterFolios: "roman", TemplateDir: "templates"}, true
	case "classic-trade":
		return Config{Name: name, Margin: "(inside: 0.85in, outside: 0.70in, top: 0.70in, bottom: 0.75in)", BodyFont: "Source Serif 4", HeadingFont: "Source Serif 4", UtilityFont: "Source Sans 3", Language: language, BodySize: "10.25pt", Leading: "14.5pt", Trim: "6x9", RunningHeads: true, FrontMatterFolios: "roman", TemplateDir: "templates"}, true
	case "timeline-trade":
		return Config{Name: name, Margin: "(inside: 0.85in, outside: 0.70in, top: 0.70in, bottom: 0.75in)", BodyFont: "Source Serif 4", HeadingFont: "Source Serif 4", UtilityFont: "Source Sans 3", Language: language, BodySize: "10pt", Leading: "15.5pt", Trim: "6x9", RunningHeads: true, FrontMatterFolios: "roman", TemplateDir: "templates/timeline-trade", PageBreakAfterThenNow: true, PageBreakAfterTimeline: true, HideTimelineHeading: true, TimelinePageCount: 2, SectionNumbering: false}, true
	default:
		return Config{}, false
	}
}

func Trade(language string) Config { cfg, _ := Preset("trade", language); return cfg }

func ApplyProject(cfg Config, project config.Project) (Config, error) {
	if project.Book.Trim != "" {
		cfg.Trim = project.Book.Trim
	}
	if project.Book.ChapterLabel != "" {
		cfg.ChapterLabel = project.Book.ChapterLabel
	}
	if project.Book.Title != "" {
		cfg.BookTitle = project.Book.Title
	}
	if project.Book.Author != "" {
		cfg.BookAuthor = project.Book.Author
	}
	if project.Book.Modified != "" {
		modified, err := time.Parse(time.RFC3339, project.Book.Modified)
		if err != nil {
			return Config{}, fmt.Errorf("book.modified must be RFC 3339: %w", err)
		}
		cfg.BookModified = modified.UTC().Format(time.RFC3339)
	}
	if value := project.Typography.BodyFont; value != "" {
		cfg.BodyFont = value
	}
	if value := project.Typography.BodySize; value != "" {
		cfg.BodySize = value
	}
	if value := project.Typography.Leading; value != "" {
		cfg.Leading = value
	}
	if value := project.Typography.HeadingFont; value != "" {
		cfg.HeadingFont = value
	}
	if value := project.Typography.UtilityFont; value != "" {
		cfg.UtilityFont = value
	}
	l := project.Layout
	if l.InsideMargin != "" && l.OutsideMargin != "" && l.TopMargin != "" && l.BottomMargin != "" {
		cfg.Margin = fmt.Sprintf("(inside: %s, outside: %s, top: %s, bottom: %s)", l.InsideMargin, l.OutsideMargin, l.TopMargin, l.BottomMargin)
	}
	if project.Pagination.RunningHeads {
		cfg.RunningHeads = true
	}
	if value := project.Pagination.FrontMatterFolios; value != "" {
		if value != "roman" && value != "none" {
			return Config{}, fmt.Errorf("unsupported front_matter_folios %q; use roman or none", value)
		}
		cfg.FrontMatterFolios = value
	}
	if project.Pagination.PageBreakAfterThenNow {
		cfg.PageBreakAfterThenNow = true
	}
	if project.Pagination.PageBreakAfterTimeline {
		cfg.PageBreakAfterTimeline = true
	}
	if project.Pagination.TimelinePageCount != 0 {
		cfg.TimelinePageCount = project.Pagination.TimelinePageCount
	}
	if project.TemplatesConfigured && project.Templates.Dir != "" {
		cfg.TemplateDir = project.Templates.Dir
	}
	if project.Fonts.Manifest != "" {
		cfg.FontManifest = project.Fonts.Manifest
		if !filepath.IsAbs(cfg.FontManifest) {
			cfg.FontManifest = filepath.Join(project.BaseDir, cfg.FontManifest)
		}
	}
	if project.Fonts.Dir != "" {
		cfg.FontDir = project.Fonts.Dir
		if !filepath.IsAbs(cfg.FontDir) {
			cfg.FontDir = filepath.Join(project.BaseDir, cfg.FontDir)
		}
	}
	if cfg.Trim != "6x9" {
		return Config{}, fmt.Errorf("unsupported trim %q; currently only 6x9 is supported", cfg.Trim)
	}
	return cfg, nil
}

func LoadFile(path, language string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var raw struct {
		Name       string            `toml:"name"`
		Typography map[string]string `toml:"typography"`
		Layout     map[string]string `toml:"layout"`
		Templates  map[string]string `toml:"templates"`
		Fonts      map[string]string `toml:"fonts"`
		Book       map[string]string `toml:"book"`
		Pagination map[string]any    `toml:"pagination"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return Config{}, err
	}
	name := raw.Name
	if name == "" {
		name = strings.TrimSuffix(strings.TrimPrefix(path, "styles/"), ".toml")
	}
	cfg, ok := Preset(name, language)
	if !ok {
		return Config{}, fmt.Errorf("unknown style %q", name)
	}
	if value := raw.Typography["body_font"]; value != "" {
		cfg.BodyFont = value
	}
	if value := raw.Typography["body_size"]; value != "" {
		cfg.BodySize = value
	}
	if value := raw.Typography["leading"]; value != "" {
		cfg.Leading = value
	}
	if value := raw.Typography["heading_font"]; value != "" {
		cfg.HeadingFont = value
	}
	if value := raw.Typography["utility_font"]; value != "" {
		cfg.UtilityFont = value
	}
	if inside, outside, top, bottom := raw.Layout["inside_margin"], raw.Layout["outside_margin"], raw.Layout["top_margin"], raw.Layout["bottom_margin"]; inside != "" && outside != "" && top != "" && bottom != "" {
		cfg.Margin = fmt.Sprintf("(inside: %s, outside: %s, top: %s, bottom: %s)", inside, outside, top, bottom)
	}
	if value := raw.Templates["dir"]; value != "" {
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		cfg.TemplateDir = filepath.Clean(value)
	}
	if value := raw.Book["chapter_label"]; value != "" {
		cfg.ChapterLabel = value
	}
	if value := raw.Book["title"]; value != "" {
		cfg.BookTitle = value
	}
	if value := raw.Book["author"]; value != "" {
		cfg.BookAuthor = value
	}
	if value := raw.Book["modified"]; value != "" {
		modified, parseErr := time.Parse(time.RFC3339, value)
		if parseErr != nil {
			return Config{}, fmt.Errorf("book.modified must be RFC 3339: %w", parseErr)
		}
		cfg.BookModified = modified.UTC().Format(time.RFC3339)
	}
	if value := raw.Fonts["manifest"]; value != "" {
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		cfg.FontManifest = filepath.Clean(value)
	}
	if value := raw.Fonts["dir"]; value != "" {
		if !filepath.IsAbs(value) {
			value = filepath.Join(filepath.Dir(path), value)
		}
		cfg.FontDir = filepath.Clean(value)
	}
	if value, ok := raw.Pagination["page_break_after_then_now"].(bool); ok {
		cfg.PageBreakAfterThenNow = value
	}
	if value, ok := raw.Pagination["page_break_after_timeline"].(bool); ok {
		cfg.PageBreakAfterTimeline = value
	}
	if value, ok := raw.Pagination["timeline_page_count"].(int64); ok {
		cfg.TimelinePageCount = int(value)
	}
	return cfg, nil
}

func ValidateTemplateDir(cfg Config) error {
	if !cfg.TemplateRequired {
		return nil
	}
	if cfg.TemplateDir == "" {
		return fmt.Errorf("style %q requires a template directory", cfg.Name)
	}
	if info, err := os.Stat(cfg.TemplateDir); err != nil || !info.IsDir() {
		return fmt.Errorf("template directory %q is not available", cfg.TemplateDir)
	}
	if _, err := os.Stat(filepath.Join(cfg.TemplateDir, "chapter.typ")); err != nil {
		return fmt.Errorf("template directory %q is missing chapter.typ", cfg.TemplateDir)
	}
	return nil
}
