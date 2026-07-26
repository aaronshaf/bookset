package artifact

import (
	"archive/zip"
	"fmt"
	"html"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/aaronshaf/bookset/internal/epub"
	"github.com/aaronshaf/bookset/internal/markdown"
)

type Issue struct{ Message string }

func (i Issue) Error() string { return i.Message }

func Validate(path string, doc *markdown.Document) []Issue {
	return ValidateDocuments(path, []*markdown.Document{doc})
}

func ValidateDocuments(path string, docs []*markdown.Document) []Issue {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return validatePDF(path, docs)
	case ".epub":
		return validateEPUB(path, docs)
	default:
		return []Issue{{fmt.Sprintf("unsupported artifact format: %s", filepath.Ext(path))}}
	}
}

func validatePDF(path string, docs []*markdown.Document) []Issue {
	var issues []Issue
	text, err := commandOutput("pdftotext", path, "-")
	if err != nil {
		return []Issue{{fmt.Sprintf("could not extract PDF text: %v", err)}}
	}
	if missing := missingFragment(text, docs); missing != "" {
		issues = append(issues, Issue{fmt.Sprintf("PDF text does not contain manuscript text: %q", missing)})
	}
	if hasInlineKind(docs, markdown.Emphasis) || hasInlineKind(docs, markdown.Strong) {
		fonts, fontErr := commandOutput("pdffonts", path)
		if fontErr != nil {
			issues = append(issues, Issue{fmt.Sprintf("could not inspect PDF fonts: %v", fontErr)})
		} else if hasInlineKind(docs, markdown.Emphasis) && !strings.Contains(strings.ToLower(fonts), "italic") && !strings.Contains(strings.ToLower(fonts), "oblique") {
			issues = append(issues, Issue{"PDF contains italic Markdown but no italic or oblique font is embedded"})
		}
	}
	info, infoErr := commandOutput("pdfinfo", path)
	if infoErr != nil {
		issues = append(issues, Issue{fmt.Sprintf("could not inspect PDF metadata: %v", infoErr)})
	} else if !regexp.MustCompile(`Page size:\s*(432 x 648|612 x 792)`).MatchString(info) {
		issues = append(issues, Issue{"PDF page size is neither the required 6x9 trim (432 x 648 points) nor a supported US Letter proof sheet (612 x 792 points)"})
	}
	return issues
}

func validateEPUB(path string, docs []*markdown.Document) []Issue {
	if err := epub.Validate(path); err != nil {
		return []Issue{{err.Error()}}
	}
	r, err := zip.OpenReader(path)
	if err != nil {
		return []Issue{{err.Error()}}
	}
	defer r.Close()
	var content strings.Builder
	for _, file := range r.File {
		if !strings.HasPrefix(file.Name, "OEBPS/content") || !strings.HasSuffix(file.Name, ".xhtml") {
			continue
		}
		reader, readErr := file.Open()
		if readErr != nil {
			return []Issue{{readErr.Error()}}
		}
		raw, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			return []Issue{{readErr.Error()}}
		}
		content.Write(raw)
	}
	contentText := content.String()
	var issues []Issue
	if missing := missingFragment(contentText, docs); missing != "" {
		issues = append(issues, Issue{"EPUB XHTML does not contain the complete manuscript body"})
	}
	if hasInlineKind(docs, markdown.Emphasis) && !strings.Contains(contentText, "<em>") {
		issues = append(issues, Issue{"EPUB lost italic markup"})
	}
	if hasInlineKind(docs, markdown.Strong) && !strings.Contains(contentText, "<strong>") {
		issues = append(issues, Issue{"EPUB lost bold markup"})
	}
	for _, doc := range docs {
		for number := range doc.Footnotes {
			if !strings.Contains(contentText, fmt.Sprintf(`id="fn-%d"`, number)) || !strings.Contains(contentText, fmt.Sprintf(`href="#fn-%d"`, number)) {
				issues = append(issues, Issue{fmt.Sprintf("EPUB footnote %d is missing a reference or definition", number)})
			}
		}
	}
	return issues
}

var tagPattern = regexp.MustCompile(`<[^>]+>`)
var tokenPattern = regexp.MustCompile(`[[:alnum:]]+`)
var pageSplitHyphenPattern = regexp.MustCompile(`-\r?\n\s*\d+\s*\r?\n`)

func missingFragment(output string, docs []*markdown.Document) string {
	output = strings.ReplaceAll(strings.ReplaceAll(output, "\u00ad\r\n", ""), "\u00ad\n", "")
	normalizedOutput := strings.ReplaceAll(strings.ReplaceAll(output, "\u00ad", ""), "-\r\n", "")
	normalizedOutput = pageSplitHyphenPattern.ReplaceAllString(normalizedOutput, "")
	normalizedOutput = strings.ReplaceAll(normalizedOutput, "-\n", "")
	text := html.UnescapeString(tagPattern.ReplaceAllString(normalizedOutput, " "))
	outputTokens := tokens(text)
	compactOutput := compact(text)
	var fragments []string
	for _, doc := range docs {
		fragments = append(fragments, textFragments(doc.Blocks)...)
	}
	if len(fragments) == 0 {
		return "no manuscript text"
	}
	for _, fragment := range fragments {
		if normalized := compact(fragment); normalized != "" && !containsTokens(outputTokens, tokens(fragment)) && !strings.Contains(compactOutput, normalized) {
			return normalized
		}
	}
	return ""
}

func tokens(value string) []string {
	matches := tokenPattern.FindAllString(strings.ToLower(value), -1)
	return matches
}

func containsTokens(haystack, needle []string) bool {
	if len(needle) == 0 {
		return true
	}
	counts := map[string]int{}
	for _, token := range haystack {
		counts[token]++
	}
	needed := map[string]int{}
	for _, token := range needle {
		needed[token]++
		if needed[token] > counts[token] {
			return false
		}
	}
	return true
}

func compact(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func textFragments(blocks []markdown.Block) []string {
	var fragments []string
	var visit func([]markdown.Block)
	visit = func(items []markdown.Block) {
		for _, block := range items {
			// Timeline is a structural Markdown marker that print templates may
			// intentionally suppress while preserving all of its entries.
			if !(block.Kind == markdown.Heading && strings.EqualFold(markdown.PlainInline(block.Inlines), "timeline")) {
				fragments = append(fragments, inlineFragments(block.Inlines)...)
			}
			visit(block.Children)
		}
	}
	visit(blocks)
	return fragments
}

func inlineFragments(inlines []markdown.Inline) []string {
	var fragments []string
	for _, in := range inlines {
		if in.Kind == markdown.Text && strings.TrimSpace(in.Text) != "" {
			fragments = append(fragments, in.Text)
		}
		fragments = append(fragments, inlineFragments(in.Children)...)
	}
	return fragments
}

func hasInlineKind(docs []*markdown.Document, wanted markdown.InlineKind) bool {
	var visitInline func([]markdown.Inline) bool
	visitInline = func(inlines []markdown.Inline) bool {
		for _, in := range inlines {
			if in.Kind == wanted || visitInline(in.Children) {
				return true
			}
		}
		return false
	}
	var visitBlock func([]markdown.Block) bool
	visitBlock = func(blocks []markdown.Block) bool {
		for _, block := range blocks {
			if visitInline(block.Inlines) || visitBlock(block.Children) {
				return true
			}
		}
		return false
	}
	for _, doc := range docs {
		if visitBlock(doc.Blocks) {
			return true
		}
	}
	return false
}

func commandOutput(name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func SortedMessages(issues []Issue) []string {
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Error())
	}
	sort.Strings(messages)
	return messages
}
