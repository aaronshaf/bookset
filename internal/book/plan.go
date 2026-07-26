package book

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aaronshaf/bookset/internal/markdown"
)

// Plan is a deterministic, renderer-independent view of a complete-book
// manifest. It is intended for preflight review before an expensive build.
type Plan struct {
	Entries []PlanEntry `json:"entries"`
	Summary PlanSummary `json:"summary"`
}

type PlanEntry struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Section   string `json:"section"`
	Chapter   int    `json:"chapter,omitempty"`
	TOC       bool   `json:"toc"`
	Source    string `json:"source,omitempty"`
	Title     string `json:"title"`
	BodyWords int    `json:"body_words"`
}

type PlanSummary struct {
	Documents int `json:"documents"`
	Chapters  int `json:"chapters"`
	Front     int `json:"front"`
	Main      int `json:"main"`
	Back      int `json:"back"`
}

// Audit verifies whole-book invariants that are meaningful only once all
// manifest entries have been loaded. It rejects duplicate source files and
// source-backed entries with no body text.
func Audit(manuscript Manuscript) (Plan, error) {
	plan := Plan{Entries: make([]PlanEntry, 0, len(manuscript.Documents))}
	sources := map[string]string{}
	var problems []string
	chapter := 0
	for _, doc := range manuscript.Documents {
		words := bodyWords(doc)
		entry := PlanEntry{ID: doc.BookID, Kind: doc.BookKind, Section: doc.PrintSection, TOC: !doc.ExcludeFromTOC, Source: doc.SourcePath, Title: doc.Title, BodyWords: words}
		if doc.BookKind == "chapter" {
			chapter++
			entry.Chapter = chapter
		}
		if doc.SourcePath != "" {
			path := filepath.Clean(doc.SourcePath)
			if first, exists := sources[path]; exists {
				problems = append(problems, fmt.Sprintf("contents entries %q and %q reference the same source %q", first, doc.BookID, path))
			} else {
				sources[path] = doc.BookID
			}
			if words == 0 {
				problems = append(problems, fmt.Sprintf("contents entry %q has no body text", doc.BookID))
			}
		}
		switch doc.PrintSection {
		case "front":
			plan.Summary.Front++
		case "main":
			plan.Summary.Main++
		case "back":
			plan.Summary.Back++
		}
		plan.Entries = append(plan.Entries, entry)
	}
	plan.Summary.Documents, plan.Summary.Chapters = len(plan.Entries), chapter
	if len(problems) > 0 {
		return plan, fmt.Errorf("manifest audit failed: %s", strings.Join(problems, "; "))
	}
	return plan, nil
}

func bodyWords(doc *markdown.Document) int {
	var text strings.Builder
	for _, block := range doc.Blocks {
		if block.Kind == markdown.Heading && block.Level == 1 {
			continue
		}
		writeBlockText(&text, block)
	}
	return len(strings.Fields(text.String()))
}

func writeBlockText(out *strings.Builder, block markdown.Block) {
	out.WriteString(markdown.PlainInline(block.Inlines))
	out.WriteByte(' ')
	for _, child := range block.Children {
		writeBlockText(out, child)
	}
}
