package typst

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/semantic"
	"github.com/aaronshaf/bookset/internal/style"
	"github.com/pelletier/go-toml/v2"
)

// Source creates deterministic Typst source. It deliberately contains no
// timestamps, random IDs, or source-path-dependent values.
func Source(doc *markdown.Document, cfg style.Config) string {
	return SourceDocuments([]*markdown.Document{doc}, cfg)
}

func SourceDocuments(docs []*markdown.Document, cfg style.Config) string {
	return sourceDocumentsFromTemplate(docs, cfg, chapterTemplate)
}

func sourceFromTemplate(doc *markdown.Document, cfg style.Config, templateText string) string {
	return sourceDocumentsFromTemplate([]*markdown.Document{doc}, cfg, templateText)
}

func sourceDocumentsFromTemplate(docs []*markdown.Document, cfg style.Config, templateText string) string {
	if len(docs) == 0 {
		return ""
	}
	doc := docs[0]
	setup := setup(doc, cfg)
	var content strings.Builder
	for i, chapter := range docs {
		if i > 0 {
			if title := chapterTitle(chapter); title != "" {
				fmt.Fprintf(&content, "#bookset-chapter.update([%s])\n", typstEscape(title))
			}
			content.WriteString("#pagebreak()\n")
		}
		normalized := semantic.Normalize(chapter, cfg)
		writeBlocks(&content, normalized.Blocks, normalized, cfg)
	}
	tmpl, err := template.New("chapter").Parse(templateText)
	if err != nil {
		return setup + "\n" + content.String()
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, makeTemplateData(setup, content.String(), doc, cfg)); err != nil {
		return setup + "\n" + content.String()
	}
	return out.String()
}

type templateData struct {
	Setup, Content, BookTitle, ChapterTitle string
	BodyFont, HeadingFont, UtilityFont      string
	BodySize, Leading                       string
}

func makeTemplateData(setupText, contentText string, doc *markdown.Document, cfg style.Config) templateData {
	bodySize := cfg.BodySize
	if bodySize == "" {
		bodySize = "10.25pt"
	}
	leading := extraLeading(cfg.BodySize, cfg.Leading)
	if leading == "" {
		leading = "4.25pt"
	}
	return templateData{
		Setup: setupText, Content: contentText, BookTitle: cfg.BookTitle, ChapterTitle: doc.Title,
		BodyFont: cfg.BodyFont, HeadingFont: cfg.HeadingFont, UtilityFont: cfg.UtilityFont,
		BodySize: bodySize, Leading: leading,
	}
}

func chapterTitle(doc *markdown.Document) string {
	for _, block := range doc.Blocks {
		if block.Kind == markdown.Heading && block.Level == 1 {
			return markdown.PlainInline(block.Inlines)
		}
	}
	return doc.Title
}

func setup(doc *markdown.Document, cfg style.Config) string {
	var b strings.Builder
	width, height := "6in", "9in"
	if cfg.Trim != "" && cfg.Trim != "6x9" {
		width, height = "6in", "9in"
	}
	margin := cfg.Margin
	if cfg.Sheet == "letter" {
		width, height = "8.5in", "11in"
		margin = "(inside: 2.10in, outside: 1.95in, top: 1.70in, bottom: 1.75in)"
	}
	fmt.Fprintf(&b, "#set page(width: %s, height: %s, margin: %s, numbering: none)\n", width, height, margin)
	if cfg.TrimMarks {
		b.WriteString(`#set page(background: context {
  let stroke = .5pt
  place(top + left, dx: 1.05in, dy: 1in)[#line(length: .2in, stroke: stroke)]
  place(top + left, dx: 1.25in, dy: .8in)[#line(angle: 90deg, length: .2in, stroke: stroke)]
  place(top + left, dx: 7.25in, dy: 1in)[#line(length: .2in, stroke: stroke)]
  place(top + left, dx: 7.25in, dy: .8in)[#line(angle: 90deg, length: .2in, stroke: stroke)]
  place(top + left, dx: 1.05in, dy: 10in)[#line(length: .2in, stroke: stroke)]
  place(top + left, dx: 1.25in, dy: 10in)[#line(angle: 90deg, length: .2in, stroke: stroke)]
  place(top + left, dx: 7.25in, dy: 10in)[#line(length: .2in, stroke: stroke)]
  place(top + left, dx: 7.25in, dy: 10in)[#line(angle: 90deg, length: .2in, stroke: stroke)]
})
`)
	}
	bodySize := cfg.BodySize
	if bodySize == "" {
		bodySize = "10.25pt"
	}
	leading := extraLeading(cfg.BodySize, cfg.Leading)
	if leading == "" {
		leading = "4.25pt"
	}
	fmt.Fprintf(&b, "#set text(font: %q, size: %s, weight: 350, tracking: -0.01em, features: (\"kern\", \"liga\", \"onum\"), costs: (widow: 50%%, orphan: 50%%), lang: %q, hyphenate: true)\n", cfg.BodyFont, bodySize, cfg.Language)
	b.WriteString("#let bookset-chapter = state(\"bookset-chapter\", [])\n")
	fmt.Fprintf(&b, "#set par(justify: true, leading: %s, first-line-indent: 0.23in, spacing: 0.60em)\n", leading)
	fmt.Fprintf(&b, "#show heading.where(level: 1): it => block(above: 1.2em, below: .6em)[#align(center)[#text(font: %q, size: 16pt)[#it.body]]]\n", cfg.HeadingFont)
	fmt.Fprintf(&b, "#show heading.where(level: 2): it => block(above: .8em, below: .4em)[#text(font: %q, size: 11pt, weight: \"bold\")[#it.body]]\n", cfg.UtilityFont)
	if cfg.RunningHeads {
		bookTitle := cfg.BookTitle
		if bookTitle == "" {
			bookTitle = doc.Title
		}
		fmt.Fprintf(&b, "#let running-head(p) = { if calc.even(p) { grid(columns: (1fr, auto, 1fr), align: (left, center, right), [#counter(page).display()], [#text(size: 7.5pt, font: %q, weight: \"medium\", tracking: 0.08em)[#upper(%q)]], []) } else { grid(columns: (1fr, auto, 1fr), align: (left, center, right), [], [#text(size: 7.5pt, font: %q, style: \"italic\")[#bookset-chapter.get()]], [#counter(page).display()]) } }\n", cfg.UtilityFont, bookTitle, cfg.HeadingFont)
		b.WriteString("#set page(header: context { let p = counter(page).get().first(); if p > 1 { running-head(p) } })\n")
	} else {
		b.WriteString("#set page(header: none, footer: none)\n")
	}
	return b.String()
}

func Render(path string, doc *markdown.Document, cfg style.Config) error {
	return RenderDocuments(path, []*markdown.Document{doc}, cfg)
}

func RenderDocuments(path string, docs []*markdown.Document, cfg style.Config) error {
	typst, err := exec.LookPath("typst")
	if err != nil {
		return fmt.Errorf("typst is required for PDF rendering: %w", err)
	}
	if err := validateConfiguredFonts(typst, cfg); err != nil {
		return err
	}
	if cfg.FontManifest != "" {
		if err := validateFonts(typst, cfg); err != nil {
			return err
		}
	}
	tmp, err := os.MkdirTemp("", "bookset-typst-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	source := filepath.Join(tmp, "book.typ")
	sourceText := SourceDocuments(docs, cfg)
	if cfg.TemplateDir != "" {
		data, readErr := os.ReadFile(filepath.Join(cfg.TemplateDir, "chapter.typ"))
		if readErr == nil {
			sourceText = sourceDocumentsFromTemplate(docs, cfg, string(data))
		} else if cfg.TemplateRequired {
			return fmt.Errorf("configured Typst template unavailable: %w", readErr)
		}
	}
	if err := os.WriteFile(source, []byte(sourceText), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	args := []string{"compile"}
	if cfg.FontDir != "" {
		args = append(args, "--font-path", cfg.FontDir)
	}
	args = append(args, "--root", tmp, source, path)
	cmd := exec.Command(typst, args...)
	cmd.Env = append(os.Environ(), "SOURCE_DATE_EPOCH=0", "LANG="+cfg.Language+"_US.UTF-8", "LC_ALL="+cfg.Language+"_US.UTF-8")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("typst compile: %w: %s", err, bytes.TrimSpace(output))
	}
	return nil
}

type fontLock struct {
	Fonts []struct {
		Family string `toml:"family"`
		Path   string `toml:"path"`
		SHA256 string `toml:"sha256"`
	} `toml:"font"`
}

func validateFonts(typstPath string, cfg style.Config) error {
	data, err := os.ReadFile(cfg.FontManifest)
	if err != nil {
		return fmt.Errorf("read font manifest: %w", err)
	}
	var lock fontLock
	if err := toml.Unmarshal(data, &lock); err != nil {
		return fmt.Errorf("parse font manifest: %w", err)
	}
	if len(lock.Fonts) == 0 {
		return fmt.Errorf("font manifest %q contains no [[font]] entries", cfg.FontManifest)
	}
	available, err := availableFonts(typstPath)
	if err != nil {
		return err
	}
	required := requiredFontFamilies(cfg)
	for family := range required {
		if family != "" && !available[family] {
			return fmt.Errorf("required font family %q is not available to Typst", family)
		}
	}
	lockedFamilies := map[string]bool{}
	manifestDir := filepath.Dir(cfg.FontManifest)
	for _, entry := range lock.Fonts {
		if entry.Family == "" || entry.Path == "" || entry.SHA256 == "" {
			return fmt.Errorf("font manifest entry must include family, path, and sha256")
		}
		if lockedFamilies[entry.Family] {
			return fmt.Errorf("font manifest contains duplicate family %q", entry.Family)
		}
		lockedFamilies[entry.Family] = true
		if !available[entry.Family] {
			return fmt.Errorf("font family %q from manifest is not available to Typst", entry.Family)
		}
		fontPath := entry.Path
		if !filepath.IsAbs(fontPath) {
			fontPath = filepath.Join(manifestDir, fontPath)
		}
		file, err := os.Open(fontPath)
		if err != nil {
			return fmt.Errorf("open font %q: %w", entry.Path, err)
		}
		hash := sha256.New()
		_, copyErr := io.Copy(hash, file)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("hash font %q: %w", entry.Path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close font %q: %w", entry.Path, closeErr)
		}
		actual := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(actual, entry.SHA256) {
			return fmt.Errorf("font checksum mismatch for %q: want %s, got %s", entry.Family, entry.SHA256, actual)
		}
	}
	for family := range required {
		if family != "" && !lockedFamilies[family] {
			return fmt.Errorf("font manifest does not lock required family %q", family)
		}
	}
	return nil
}

func validateConfiguredFonts(typstPath string, cfg style.Config) error {
	available, err := availableFonts(typstPath)
	if err != nil {
		return err
	}
	for family := range requiredFontFamilies(cfg) {
		if family != "" && !available[family] {
			return fmt.Errorf("required font family %q is not available to Typst", family)
		}
	}
	return nil
}

func availableFonts(typstPath string) (map[string]bool, error) {
	output, err := exec.Command(typstPath, "fonts").Output()
	if err != nil {
		return nil, fmt.Errorf("list Typst fonts: %w", err)
	}
	available := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		if family := strings.TrimSpace(line); family != "" {
			available[family] = true
		}
	}
	return available, nil
}

func requiredFontFamilies(cfg style.Config) map[string]bool {
	return map[string]bool{cfg.BodyFont: true, cfg.HeadingFont: true, cfg.UtilityFont: true}
}

const chapterTemplate = `{{.Setup}}
{{.Content}}
`

func writeBlocks(b *strings.Builder, blocks []semantic.Block, doc semantic.Document, cfg style.Config) {
	for _, block := range blocks {
		writeSemanticBlock(b, block, doc, cfg)
	}
}

func writeSemanticBlock(b *strings.Builder, block semantic.Block, doc semantic.Document, cfg style.Config) {
	switch block.Kind {
	case semantic.ChapterOpener:
		fmt.Fprintf(b, "#bookset-chapter.update([%s])\n", typstEscape(markdown.PlainInline(block.Inlines)))
		fmt.Fprintf(b, "#chapter-title([%s], [%s])\n", inline(block.Inlines, doc), typstEscape(block.Label))
	case semantic.ThenNow:
		if block.Label == "NOW" {
			b.WriteString("#v(0.55em)\n")
		}
		fmt.Fprintf(b, "#then-now([%s], [%s])\n", block.Label, inline(block.Inlines, doc))
		if block.Label == "NOW" && cfg.PageBreakAfterThenNow {
			b.WriteString("#pagebreak()\n")
		}
	case semantic.Section:
		fmt.Fprintf(b, "= %s. %s\n", roman(block.Number), inline(block.Inlines, doc))
	case semantic.Timeline:
		for i, item := range block.Children {
			if i > 0 {
				b.WriteString("#v(0.25em)\n")
			}
			fmt.Fprintf(b, "#timeline-item([%s], [%s])\n", typstEscape(item.Date), inline(item.Inlines, doc))
		}
		if cfg.PageBreakAfterTimeline {
			b.WriteString("#pagebreak()\n")
		}
	default:
		writeBlock(b, block, doc, cfg)
	}
}

func roman(number int) string {
	values := []struct {
		value int
		glyph string
	}{{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"}}
	var out strings.Builder
	for _, item := range values {
		count := number / item.value
		number %= item.value
		out.WriteString(strings.Repeat(item.glyph, count))
	}
	return out.String()
}

func extraLeading(body, leading string) string {
	parse := func(value string) (float64, bool) {
		match := regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)pt$`).FindStringSubmatch(strings.TrimSpace(value))
		if match == nil {
			return 0, false
		}
		number, err := strconv.ParseFloat(match[1], 64)
		return number, err == nil
	}
	bodyValue, bodyOK := parse(body)
	leadingValue, leadingOK := parse(leading)
	if bodyOK && leadingOK && leadingValue > bodyValue {
		return strconv.FormatFloat(leadingValue-bodyValue, 'f', -1, 64) + "pt"
	}
	return leading
}

func writeBlock(b *strings.Builder, block semantic.Block, doc semantic.Document, cfg style.Config) {
	switch block.Kind {
	case semantic.Heading:
		if block.Level == 1 {
			fmt.Fprintf(b, "#bookset-chapter.update([%s])\n", typstEscape(markdown.PlainInline(block.Inlines)))
		}
		fmt.Fprintf(b, "%s %s\n", strings.Repeat("=", block.Level), inline(block.Inlines, doc))
	case semantic.Paragraph:
		fmt.Fprintf(b, "#par(first-line-indent: 0.23in)[%s]\n", inline(block.Inlines, doc))
	case semantic.Quote:
		b.WriteString("#block(breakable: false, inset: (left: 0.35in, right: 0.30in), above: .9em, below: .9em)[")
		for _, child := range block.Children {
			writeBlock(b, child, doc, cfg)
		}
		b.WriteString("]\n")
	case semantic.List:
		for i, child := range block.Children {
			marker := "-"
			if block.Ordered {
				marker = fmt.Sprintf("%d.", block.Start+i)
			}
			fmt.Fprintf(b, "%s %s\n", marker, inline(child.Inlines, doc))
		}
	}
}

func inline(inlines []markdown.Inline, doc semantic.Document) string {
	var b strings.Builder
	for _, in := range inlines {
		switch in.Kind {
		case markdown.Text:
			b.WriteString(typstEscape(in.Text))
		case markdown.Emphasis:
			b.WriteString("#emph[")
			b.WriteString(inline(in.Children, doc))
			b.WriteByte(']')
		case markdown.Strong:
			b.WriteString("#strong[")
			b.WriteString(inline(in.Children, doc))
			b.WriteByte(']')
		case markdown.CodeSpan:
			b.WriteString(`#raw("`)
			b.WriteString(typstRawString(markdown.PlainInline(in.Children)))
			b.WriteString(`")`)
		case markdown.Footnote:
			b.WriteString("#footnote[")
			b.WriteString(inline(doc.Footnotes[in.Number], doc))
			b.WriteByte(']')
		}
	}
	return b.String()
}

func typstEscape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	for _, char := range []string{"#", "[", "]", "*", "_", "$"} {
		value = strings.ReplaceAll(value, char, "\\"+char)
	}
	return value
}

func typstRawString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
