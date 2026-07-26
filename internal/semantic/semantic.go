// Package semantic turns ordinary Markdown blocks into project-level book
// structures. It is intentionally presentation-neutral: PDF and EPUB
// renderers consume the same normalized nodes.
package semantic

import (
	"strings"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
)

type Kind string

const (
	Heading       Kind = "heading"
	Paragraph     Kind = "paragraph"
	Quote         Kind = "quote"
	List          Kind = "list"
	ChapterOpener Kind = "chapter-opener"
	ThenNow       Kind = "then-now"
	Section       Kind = "section"
	Timeline      Kind = "timeline"
	TimelineItem  Kind = "timeline-item"
)

type Document struct {
	Title     string
	Author    string
	Language  string
	Footnotes map[int][]markdown.Inline
	Blocks    []Block
}

type Block struct {
	Kind     Kind
	Level    int
	Inlines  []markdown.Inline
	Children []Block
	Ordered  bool
	Start    int
	Label    string
	Number   int
	Date     string
}

func Normalize(doc *markdown.Document, cfg style.Config) Document {
	out := Document{Title: doc.Title, Author: doc.Author, Language: doc.Language, Footnotes: doc.Footnotes}
	firstHeading := true
	inTimeline := false
	sectionNumber := 0
	for _, block := range doc.Blocks {
		if block.Kind == markdown.Heading {
			title := markdown.PlainInline(block.Inlines)
			if inTimeline {
				inTimeline = false
			}
			if firstHeading && block.Level == 1 && cfg.ChapterLabel != "" {
				out.Blocks = append(out.Blocks, Block{Kind: ChapterOpener, Level: block.Level, Inlines: block.Inlines, Label: cfg.ChapterLabel})
				firstHeading = false
				continue
			}
			firstHeading = false
			if strings.EqualFold(title, "Timeline") && cfg.HideTimelineHeading {
				inTimeline = true
				continue
			}
			if cfg.SectionNumbering && block.Level == 2 {
				sectionNumber++
				out.Blocks = append(out.Blocks, Block{Kind: Section, Level: block.Level, Inlines: block.Inlines, Number: sectionNumber})
				continue
			}
			out.Blocks = append(out.Blocks, normalizeBlock(block))
			continue
		}
		if inTimeline && block.Kind == markdown.List {
			items := make([]Block, 0, len(block.Children))
			for _, item := range block.Children {
				date, body := splitTimeline(item.Inlines)
				items = append(items, Block{Kind: TimelineItem, Inlines: body, Date: date})
			}
			out.Blocks = append(out.Blocks, Block{Kind: Timeline, Children: items, Ordered: block.Ordered, Start: block.Start})
			inTimeline = false
			continue
		}
		if cfg.PageBreakAfterThenNow && block.Kind == markdown.Paragraph {
			if label, rest, ok := thenNow(block); ok {
				out.Blocks = append(out.Blocks, Block{Kind: ThenNow, Inlines: rest, Label: label})
				continue
			}
		}
		out.Blocks = append(out.Blocks, normalizeBlock(block))
	}
	return out
}

func normalizeBlock(block markdown.Block) Block {
	out := Block{Kind: Kind(block.Kind), Level: block.Level, Inlines: block.Inlines, Ordered: block.Ordered, Start: block.Start}
	for _, child := range block.Children {
		out.Children = append(out.Children, normalizeBlock(child))
	}
	return out
}

func thenNow(block markdown.Block) (string, []markdown.Inline, bool) {
	if len(block.Inlines) == 0 || block.Inlines[0].Kind != markdown.Strong {
		return "", nil, false
	}
	label := strings.ToUpper(strings.TrimSuffix(strings.TrimSpace(markdown.PlainInline(block.Inlines[0].Children)), ":"))
	if label != "THEN" && label != "NOW" {
		return "", nil, false
	}
	return label, block.Inlines[1:], true
}

func splitTimeline(inlines []markdown.Inline) (string, []markdown.Inline) {
	if len(inlines) == 0 {
		return "", nil
	}
	if inlines[0].Kind != markdown.Strong {
		return "", inlines
	}
	date := strings.TrimSuffix(strings.TrimSpace(markdown.PlainInline(inlines[0].Children)), ":")
	return date, inlines[1:]
}
