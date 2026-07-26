package markdown

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	mdast "github.com/yuin/goldmark/extension/ast"
	textm "github.com/yuin/goldmark/text"
)

type Issue struct{ Message string }

func (i Issue) Error() string { return i.Message }

var frontMatter = regexp.MustCompile(`(?m)^([a-zA-Z]+):\s*["']?([^"']*)["']?\s*$`)
var footnoteReference = regexp.MustCompile(`\[\^([^\]]+)\]`)
var footnoteDefinition = regexp.MustCompile(`(?m)^\[\^([^\]]+)\]:`)

func Parse(source []byte) (*Document, []Issue) {
	body, metadata := stripFrontMatter(source)
	doc := &Document{Title: metadata["title"], Author: metadata["author"], Language: metadata["language"], Footnotes: map[int][]Inline{}}
	root := goldmark.New(goldmark.WithExtensions(extension.Footnote)).Parser().Parse(textm.NewReader(body))
	var issues []Issue
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		block, ok := parseBlock(node, body, doc, &issues)
		if ok {
			doc.Blocks = append(doc.Blocks, block)
		}
	}
	collectFootnotes(root, body, doc, &issues)
	if integrityText(astText(root, body)) != integrityText(doc.PlainText()) {
		issues = append(issues, Issue{"source text was dropped or duplicated while building the document model"})
	}
	if doc.Title == "" {
		for _, block := range doc.Blocks {
			if block.Kind == Heading && block.Level == 1 {
				doc.Title = inlineText(block.Inlines)
				break
			}
		}
	}
	defined := map[string]bool{}
	for _, match := range footnoteDefinition.FindAllSubmatch(body, -1) {
		defined[string(match[1])] = true
	}
	referenced := map[string]bool{}
	var referenceSource strings.Builder
	for _, line := range strings.Split(string(body), "\n") {
		if footnoteDefinition.MatchString(line) {
			continue
		}
		referenceSource.WriteString(line)
		referenceSource.WriteByte('\n')
	}
	for _, match := range footnoteReference.FindAllSubmatch([]byte(referenceSource.String()), -1) {
		name := string(match[1])
		referenced[name] = true
		if !defined[name] {
			issues = append(issues, Issue{fmt.Sprintf("undefined footnote: %s", match[1])})
		}
	}
	for name := range defined {
		if !referenced[name] {
			issues = append(issues, Issue{fmt.Sprintf("unused footnote: %s", name)})
		}
	}
	return doc, issues
}

func compactText(value string) string { return strings.Join(strings.Fields(value), "") }

func integrityText(value string) string {
	value = html.UnescapeString(value)
	value = strings.ReplaceAll(value, `\<`, "<")
	value = strings.ReplaceAll(value, `\>`, ">")
	return compactText(value)
}

func astText(node ast.Node, source []byte) string {
	var out strings.Builder
	_ = ast.Walk(node, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := child.(type) {
		case *ast.Text:
			out.Write(n.Segment.Value(source))
			if n.SoftLineBreak() {
				out.WriteByte(' ')
			}
		case *ast.String:
			out.Write(n.Value)
		case *ast.RawHTML:
			out.Write(n.Segments.Value(source))
		case *mdast.FootnoteList:
			return ast.WalkSkipChildren, nil
		case *mdast.FootnoteLink, *mdast.FootnoteBacklink:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return out.String()
}

func stripFrontMatter(source []byte) ([]byte, map[string]string) {
	metadata := map[string]string{}
	if !bytes.HasPrefix(source, []byte("---\n")) {
		return source, metadata
	}
	end := bytes.Index(source[4:], []byte("\n---"))
	if end < 0 {
		return source, metadata
	}
	end += 4
	for _, match := range frontMatter.FindAllStringSubmatch(string(source[4:end]), -1) {
		metadata[strings.ToLower(match[1])] = strings.TrimSpace(match[2])
	}
	return source[end+4:], metadata
}

func parseBlock(node ast.Node, source []byte, doc *Document, issues *[]Issue) (Block, bool) {
	switch n := node.(type) {
	case *ast.Heading:
		return Block{Kind: Heading, Level: n.Level, Inlines: parseInlines(n, source, doc, issues)}, true
	case *ast.Paragraph:
		return Block{Kind: Paragraph, Inlines: parseInlines(n, source, doc, issues)}, true
	case *ast.TextBlock:
		return Block{Kind: Paragraph, Inlines: parseInlines(n, source, doc, issues)}, true
	case *ast.Blockquote:
		return parseContainer(Quote, n, source, doc, issues), true
	case *ast.List:
		b := parseContainer(List, n, source, doc, issues)
		b.Ordered, b.Start = n.IsOrdered(), n.Start
		return b, true
	case *ast.ListItem:
		var inlines []Inline
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			if paragraph, ok := child.(*ast.TextBlock); ok {
				inlines = append(inlines, parseInlines(paragraph, source, doc, issues)...)
				continue
			}
			if paragraph, ok := child.(*ast.Paragraph); ok {
				inlines = append(inlines, parseInlines(paragraph, source, doc, issues)...)
				continue
			}
			*issues = append(*issues, Issue{fmt.Sprintf("unsupported list item construct: %s", child.Kind())})
		}
		return Block{Kind: ListItem, Inlines: inlines}, true
	case *ast.ThematicBreak:
		return Block{Kind: ThematicBreak}, true
	case *mdast.FootnoteList:
		return Block{}, true
	default:
		*issues = append(*issues, Issue{fmt.Sprintf("unsupported Markdown construct: %s", node.Kind())})
		return Block{}, false
	}
}

func parseContainer(kind BlockKind, node ast.Node, source []byte, doc *Document, issues *[]Issue) Block {
	b := Block{Kind: kind}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Kind() == mdast.KindFootnoteList {
			continue
		}
		parsed, ok := parseBlock(child, source, doc, issues)
		if ok {
			b.Children = append(b.Children, parsed)
		}
	}
	return b
}

func parseInlines(node ast.Node, source []byte, doc *Document, issues *[]Issue) []Inline {
	var out []Inline
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			out = appendText(out, string(n.Segment.Value(source)))
			if n.SoftLineBreak() {
				out = append(out, Inline{Kind: Text, Text: " "})
			}
		case *ast.Emphasis:
			kind := Emphasis
			if n.Level == 2 {
				kind = Strong
			}
			out = append(out, Inline{Kind: kind, Children: parseInlines(n, source, doc, issues)})
		case *ast.CodeSpan:
			out = append(out, Inline{Kind: CodeSpan, Children: parseInlines(n, source, doc, issues)})
		case *mdast.FootnoteLink:
			out = append(out, Inline{Kind: Footnote, Number: n.Index})
		case *mdast.FootnoteBacklink:
			// Backlinks are generated metadata, not manuscript text.
		case *ast.String:
			out = appendText(out, string(n.Value))
		case *ast.RawHTML:
			raw := string(n.Segments.Value(source))
			// Goldmark recognizes angle-bracket transcription as RawHTML even
			// when the opening bracket was backslash-escaped. Preserve it as
			// literal manuscript text, never executable markup.
			if len(out) > 0 && out[len(out)-1].Kind == Text && strings.HasSuffix(out[len(out)-1].Text, "\\") && strings.HasPrefix(raw, "<") {
				out[len(out)-1].Text = strings.TrimSuffix(out[len(out)-1].Text, "\\")
			}
			out = appendText(out, raw)
		default:
			*issues = append(*issues, Issue{fmt.Sprintf("unsupported inline construct: %s", child.Kind())})
		}
	}
	return out
}

func appendText(inlines []Inline, value string) []Inline {
	return append(inlines, Inline{Kind: Text, Text: html.UnescapeString(value)})
}

// collectFootnotes extracts definitions after parsing because goldmark places
// them in a dedicated list node. It is kept separate to keep rendering simple.
func collectFootnotes(root ast.Node, source []byte, doc *Document, issues *[]Issue) {
	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		if fn, ok := node.(*mdast.FootnoteList); ok {
			for child := fn.FirstChild(); child != nil; child = child.NextSibling() {
				footnote := child.(*mdast.Footnote)
				id := footnote.Index
				for body := footnote.FirstChild(); body != nil; body = body.NextSibling() {
					if parsed, ok := parseBlock(body, source, doc, issues); ok {
						doc.Footnotes[id] = parsed.Inlines
					}
				}
			}
		}
	}
}
