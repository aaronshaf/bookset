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
	Documents []*markdown.Document
	Chapters  []*markdown.Document
	Style     style.Config
}

func Load(project config.Project) (Manuscript, error) {
	contents, err := orderedContents(project)
	if err != nil {
		return Manuscript{}, err
	}
	var result Manuscript
	chapterNumber := 0
	tocCount := 0
	previousSection := -1
	seenIDs := map[string]bool{}
	for index, entry := range contents {
		if entry.ID == "" {
			return Manuscript{}, fmt.Errorf("contents entry %d has no id", index+1)
		}
		if seenIDs[entry.ID] {
			return Manuscript{}, fmt.Errorf("contents entry has duplicate id %q", entry.ID)
		}
		seenIDs[entry.ID] = true
		if entry.Kind == "" {
			entry.Kind = "chapter"
		}
		if !validKind(entry.Kind) {
			return Manuscript{}, fmt.Errorf("contents entry %q has unsupported kind %q", entry.ID, entry.Kind)
		}
		section, sectionErr := resolvedPrintSection(entry.PrintSection, entry.Kind)
		if sectionErr != nil {
			return Manuscript{}, fmt.Errorf("contents entry %q: %w", entry.ID, sectionErr)
		}
		if rank := printSectionRank(section); rank < previousSection {
			return Manuscript{}, fmt.Errorf("contents entry %q moves from %s back to %s; print sections must remain ordered", entry.ID, printSectionName(previousSection), section)
		} else {
			previousSection = rank
		}
		var doc *markdown.Document
		if entry.Kind == "part" {
			if entry.Source != "" || entry.Title == "" {
				return Manuscript{}, fmt.Errorf("part %q requires title and must not set source", entry.ID)
			}
			doc = &markdown.Document{Title: entry.Title}
		} else if entry.Kind == "toc" {
			if entry.Source != "" {
				return Manuscript{}, fmt.Errorf("toc %q must not set source", entry.ID)
			}
			tocCount++
			if tocCount > 1 {
				return Manuscript{}, fmt.Errorf("book config contains more than one toc entry")
			}
			title := entry.Title
			if title == "" {
				title = "Contents"
			}
			doc = &markdown.Document{Title: title}
		} else {
			if entry.Source == "" {
				return Manuscript{}, fmt.Errorf("%s %q has no source", entry.Kind, entry.ID)
			}
			source, readErr := os.ReadFile(filepath.Clean(entry.Source))
			if readErr != nil {
				return Manuscript{}, fmt.Errorf("contents entry %q: %w", entry.ID, readErr)
			}
			var parseIssues []markdown.Issue
			doc, parseIssues = markdown.Parse(source)
			doc.SourcePath = filepath.Clean(entry.Source)
			if len(parseIssues) > 0 {
				return Manuscript{}, fmt.Errorf("contents entry %q: %s", entry.ID, markdown.FormatIssues(parseIssues))
			}
		}
		doc.BookID, doc.BookKind, doc.PrintSection = entry.ID, entry.Kind, section
		doc.ExcludeFromTOC = entry.Kind == "toc" || !tocDefault(entry.Kind, entry.TOC)
		if entry.Title != "" {
			doc.Title = entry.Title
		}
		if project.Book.Language != "" {
			doc.Language = project.Book.Language
		}
		if project.Book.Title != "" && index == 0 && doc.Title == "" {
			doc.Title = project.Book.Title
		}
		cfg, err := chapterStyle(entry.Style, doc.Language, project)
		if err != nil {
			return Manuscript{}, fmt.Errorf("contents entry %q: %w", entry.ID, err)
		}
		if index == 0 {
			result.Style = cfg
		} else if cfg.Name != result.Style.Name || cfg.TemplateDir != result.Style.TemplateDir {
			return Manuscript{}, fmt.Errorf("contents entry %q uses a different style; mixed styles are not supported in one build", entry.ID)
		}
		if issues := markdown.Validate(doc, nil); len(issues) > 0 {
			return Manuscript{}, fmt.Errorf("contents entry %q: %s", entry.ID, markdown.FormatIssues(issues))
		}
		if entry.Kind == "chapter" {
			chapterNumber++
			doc.ChapterLabel = resolvedChapterLabel(entry.ChapterLabel, cfg.ChapterLabel, project.Book.ChapterNumbering, chapterNumber)
			result.Chapters = append(result.Chapters, doc)
		}
		result.Documents = append(result.Documents, doc)
	}
	return result, nil
}

func orderedContents(project config.Project) ([]config.Content, error) {
	if len(project.Contents) > 0 && len(project.Chapters) > 0 {
		return nil, fmt.Errorf("book config cannot contain both [[contents]] and [[chapters]]")
	}
	if len(project.Contents) > 0 {
		return project.Contents, nil
	}
	if len(project.Chapters) == 0 {
		return nil, fmt.Errorf("book config contains no [[contents]] or [[chapters]] entries")
	}
	contents := make([]config.Content, 0, len(project.Chapters))
	for index, chapter := range project.Chapters {
		contents = append(contents, config.Content{ID: fmt.Sprintf("chapter-%03d", index+1), Kind: "chapter", Source: chapter.Source, Style: chapter.Style, ChapterLabel: chapter.ChapterLabel})
	}
	return contents, nil
}

func validKind(kind string) bool {
	return kind == "front-matter" || kind == "part" || kind == "chapter" || kind == "back-matter" || kind == "toc"
}

func resolvedPrintSection(value, kind string) (string, error) {
	if value != "" {
		if value == "front" || value == "main" || value == "back" {
			return value, nil
		}
		return "", fmt.Errorf("unsupported print_section %q; use front, main, or back", value)
	}
	if kind == "front-matter" || kind == "toc" {
		return "front", nil
	}
	if kind == "back-matter" {
		return "back", nil
	}
	return "main", nil
}
func printSectionRank(section string) int {
	if section == "front" {
		return 0
	}
	if section == "main" {
		return 1
	}
	return 2
}
func printSectionName(rank int) string { return []string{"front", "main", "back"}[rank] }
func tocDefault(kind string, value *bool) bool {
	if value != nil {
		return *value
	}
	return kind == "part" || kind == "chapter"
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
