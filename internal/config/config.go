package config

import (
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

type Project struct {
	Book                Book       `toml:"book"`
	Typography          Typography `toml:"typography"`
	Layout              Layout     `toml:"layout"`
	Pagination          Pagination `toml:"pagination"`
	Templates           Templates  `toml:"templates"`
	Fonts               Fonts      `toml:"fonts"`
	BaseDir             string     `toml:"-"`
	Chapters            []Chapter  `toml:"chapters"`
	Contents            []Content  `toml:"contents"`
	TemplatesConfigured bool       `toml:"-"`
}

type Book struct {
	Title            string `toml:"title"`
	Author           string `toml:"author"`
	Modified         string `toml:"modified"`
	Cover            string `toml:"cover"`
	CoverAlt         string `toml:"cover_alt"`
	Language         string `toml:"language"`
	Trim             string `toml:"trim"`
	ChapterLabel     string `toml:"chapter_label"`
	ChapterNumbering bool   `toml:"chapter_numbering"`
}
type Typography struct {
	BodyFont    string `toml:"body_font"`
	BodySize    string `toml:"body_size"`
	Leading     string `toml:"leading"`
	HeadingFont string `toml:"heading_font"`
	UtilityFont string `toml:"utility_font"`
}
type Layout struct {
	InsideMargin  string `toml:"inside_margin"`
	OutsideMargin string `toml:"outside_margin"`
	TopMargin     string `toml:"top_margin"`
	BottomMargin  string `toml:"bottom_margin"`
}
type Pagination struct {
	TimelinePageCount      int    `toml:"timeline_page_count"`
	PageBreakAfterThenNow  bool   `toml:"page_break_after_then_now"`
	PageBreakAfterTimeline bool   `toml:"page_break_after_timeline"`
	RunningHeads           bool   `toml:"running_heads"`
	FrontMatterFolios      string `toml:"front_matter_folios"`
}

type Templates struct {
	Dir string `toml:"dir"`
}

type Fonts struct {
	Manifest string `toml:"manifest"`
	Dir      string `toml:"dir"`
}

type Chapter struct {
	Source       string `toml:"source"`
	Style        string `toml:"style"`
	ChapterLabel string `toml:"chapter_label"`
}

// Content is an ordered book-sequence entry. It supersedes Chapter for new
// manifests while Chapter remains supported for compatibility.
type Content struct {
	ID           string `toml:"id"`
	Kind         string `toml:"kind"`
	Source       string `toml:"source"`
	Style        string `toml:"style"`
	Title        string `toml:"title"`
	ChapterLabel string `toml:"chapter_label"`
	TOC          *bool  `toml:"toc"`
	PrintSection string `toml:"print_section"`
}

func Load(path string) (Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Project{}, err
	}
	var project Project
	err = toml.Unmarshal(data, &project)
	if err == nil {
		absolute, absoluteErr := filepath.Abs(path)
		if absoluteErr != nil {
			return Project{}, absoluteErr
		}
		project.BaseDir = filepath.Dir(absolute)
		project.Book.Cover = resolvePath(project.BaseDir, project.Book.Cover)
		project.TemplatesConfigured = project.Templates.Dir != ""
		if project.Templates.Dir == "" {
			project.Templates.Dir = filepath.Join(project.BaseDir, "templates")
		} else if !filepath.IsAbs(project.Templates.Dir) {
			project.Templates.Dir = filepath.Clean(filepath.Join(project.BaseDir, project.Templates.Dir))
		}
		for i := range project.Chapters {
			project.Chapters[i].Source = resolvePath(project.BaseDir, project.Chapters[i].Source)
			project.Chapters[i].Style = resolveStylePath(project.BaseDir, project.Chapters[i].Style)
		}
		for i := range project.Contents {
			project.Contents[i].Source = resolvePath(project.BaseDir, project.Contents[i].Source)
			project.Contents[i].Style = resolveStylePath(project.BaseDir, project.Contents[i].Style)
		}
	}
	return project, err
}

func resolvePath(baseDir, value string) string {
	if value == "" || filepath.IsAbs(value) {
		return value
	}
	return filepath.Join(baseDir, value)
}

func resolveStylePath(baseDir, value string) string {
	if value == "" || filepath.IsAbs(value) || (filepath.Ext(value) != ".toml" && filepath.Dir(value) == ".") {
		return value
	}
	return filepath.Join(baseDir, value)
}
