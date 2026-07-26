package markdown

import (
	"fmt"
	"strings"
)

// Validate checks the semantic invariants that renderers rely on.
func Validate(doc *Document, parseIssues []Issue) []Issue {
	issues := append([]Issue(nil), parseIssues...)
	used := map[int]bool{}
	var visitInline func([]Inline)
	visitInline = func(inlines []Inline) {
		for _, in := range inlines {
			if in.Kind == Footnote {
				used[in.Number] = true
			}
			visitInline(in.Children)
		}
	}
	var visitBlock func([]Block)
	visitBlock = func(blocks []Block) {
		for _, block := range blocks {
			visitInline(block.Inlines)
			visitBlock(block.Children)
		}
	}
	visitBlock(doc.Blocks)
	for number := range used {
		if _, ok := doc.Footnotes[number]; !ok {
			issues = append(issues, Issue{fmt.Sprintf("undefined footnote: %d", number)})
		}
	}
	for number := range doc.Footnotes {
		if !used[number] {
			issues = append(issues, Issue{fmt.Sprintf("unused footnote: %d", number)})
		}
	}
	return issues
}

func FormatIssues(issues []Issue) string {
	parts := make([]string, 0, len(issues))
	for _, issue := range issues {
		parts = append(parts, issue.Error())
	}
	return strings.Join(parts, "; ")
}
