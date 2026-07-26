package book

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aaronshaf/bookset/internal/config"
	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
)

type Manuscript struct {
	Chapters []*markdown.Document
	Style    style.Config
}

func Load(project config.Project) (Manuscript, error) {
	if len(project.Chapters) == 0 {
		return Manuscript{}, fmt.Errorf("book config contains no [[chapters]] entries")
	}
	var result Manuscript
	for i, chapter := range project.Chapters {
		if chapter.Source == "" {
			return Manuscript{}, fmt.Errorf("chapter %d has no source", i+1)
		}
		source, err := os.ReadFile(filepath.Clean(chapter.Source))
		if err != nil {
			return Manuscript{}, fmt.Errorf("chapter %q: %w", chapter.Source, err)
		}
		doc, parseIssues := markdown.Parse(source)
		if len(parseIssues) > 0 {
			return Manuscript{}, fmt.Errorf("chapter %q: %s", chapter.Source, markdown.FormatIssues(parseIssues))
		}
		if project.Book.Language != "" {
			doc.Language = project.Book.Language
		}
		if project.Book.Title != "" && i == 0 {
			doc.Title = project.Book.Title
		}
		cfg, err := chapterStyle(chapter.Style, doc.Language, project)
		if err != nil {
			return Manuscript{}, fmt.Errorf("chapter %q: %w", chapter.Source, err)
		}
		if i == 0 {
			result.Style = cfg
		} else if cfg.Name != result.Style.Name || cfg.TemplateDir != result.Style.TemplateDir {
			return Manuscript{}, fmt.Errorf("chapter %q uses a different style; mixed chapter styles are not supported in one build", chapter.Source)
		}
		if issues := markdown.Validate(doc, nil); len(issues) > 0 {
			return Manuscript{}, fmt.Errorf("chapter %q: %s", chapter.Source, markdown.FormatIssues(issues))
		}
		doc.ChapterLabel = resolvedChapterLabel(chapter.ChapterLabel, cfg.ChapterLabel, project.Book.ChapterNumbering, i+1)
		result.Chapters = append(result.Chapters, doc)
	}
	return result, nil
}

func resolvedChapterLabel(override, defaultLabel string, numbered bool, number int) string {
	if override != "" {
		return override
	}
	if defaultLabel == "" {
		return ""
	}
	if numbered {
		return fmt.Sprintf("%s %d", defaultLabel, number)
	}
	return defaultLabel
}

func chapterStyle(name, language string, project config.Project) (style.Config, error) {
	if name == "" {
		name = "trade"
	}
	fileStyle := strings.HasSuffix(name, ".toml") || strings.Contains(name, string(os.PathSeparator))
	var cfg style.Config
	var ok bool
	var err error
	if fileStyle {
		cfg, err = style.LoadFile(name, language)
	} else {
		cfg, ok = style.Preset(name, language)
		if !ok {
			return style.Config{}, fmt.Errorf("unknown style %q", name)
		}
	}
	if err != nil {
		return style.Config{}, err
	}
	if !fileStyle && !filepath.IsAbs(cfg.TemplateDir) {
		cfg.TemplateDir = filepath.Join(project.BaseDir, cfg.TemplateDir)
	}
	if project.Book.Title != "" {
		cfg.BookTitle = project.Book.Title
	}
	cfg, err = style.ApplyProject(cfg, project)
	if err != nil {
		return style.Config{}, err
	}
	cfg.TemplateRequired = fileStyle || project.TemplatesConfigured
	if err := style.ValidateTemplateDir(cfg); err != nil {
		return style.Config{}, err
	}
	return cfg, nil
}
