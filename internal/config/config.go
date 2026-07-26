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
	TemplatesConfigured bool       `toml:"-"`
}

type Book struct {
	Title            string `toml:"title"`
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
	TimelinePageCount      int  `toml:"timeline_page_count"`
	PageBreakAfterThenNow  bool `toml:"page_break_after_then_now"`
	PageBreakAfterTimeline bool `toml:"page_break_after_timeline"`
	RunningHeads           bool `toml:"running_heads"`
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
		project.TemplatesConfigured = project.Templates.Dir != ""
		if project.Templates.Dir == "" {
			project.Templates.Dir = filepath.Join(project.BaseDir, "templates")
		} else if !filepath.IsAbs(project.Templates.Dir) {
			project.Templates.Dir = filepath.Clean(filepath.Join(project.BaseDir, project.Templates.Dir))
		}
		for i := range project.Chapters {
			if !filepath.IsAbs(project.Chapters[i].Source) {
				project.Chapters[i].Source = filepath.Join(project.BaseDir, project.Chapters[i].Source)
			}
			if project.Chapters[i].Style != "" && !filepath.IsAbs(project.Chapters[i].Style) && (filepath.Ext(project.Chapters[i].Style) == ".toml" || filepath.Dir(project.Chapters[i].Style) != ".") {
				project.Chapters[i].Style = filepath.Join(project.BaseDir, project.Chapters[i].Style)
			}
		}
	}
	return project, err
}
