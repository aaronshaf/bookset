package typst

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
// timestamps or random IDs. Source markers are comments used only to map
// diagnostics back to Markdown.
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
	hasPreamble := writeBookPreamble(&content, cfg)
	for i, chapter := range docs {
		if i > 0 || hasPreamble {
			if chapter.BookKind == "" || chapter.BookKind == "chapter" {
				if title := chapterTitle(chapter); title != "" {
					fmt.Fprintf(&content, "#bookset-chapter.update([%s])\n", typstEscape(title))
				}
			}
			content.WriteString("#pagebreak()\n")
		}
		if i == 0 || chapter.PrintSection != docs[i-1].PrintSection {
			writePrintSection(&content, chapter.PrintSection, cfg)
		}
		if chapter.BookID != "" && !chapter.ExcludeFromTOC && chapter.BookKind != "toc" {
			fmt.Fprintf(&content, "#label(%q)\n", tocLabel(chapter.BookID))
			writeBookmark(&content, docs, i, chapter)
		}
		writeProofMarker(&content, chapter)
		if chapter.BookKind == "toc" {
			writeTOC(&content, docs, chapter, cfg)
			continue
		}
		normalized := semantic.Normalize(chapter, cfg)
		writeBlocks(&content, normalized.Blocks, normalized, cfg)
	}
	content.WriteString(`#context [#metadata((page: here().page())) <bookset-proof-end>]` + "\n")
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

func writeProofMarker(content *strings.Builder, doc *markdown.Document) {
	fmt.Fprintf(content, "#context [#metadata((id: %q, kind: %q, section: %q, title: %q, page: here().page(), folio: counter(page).get().first())) <bookset-entry>]\n", doc.BookID, doc.BookKind, doc.PrintSection, doc.Title)
}

func writeBookPreamble(content *strings.Builder, cfg style.Config) bool {
	if cfg.CoverPath == "" && !cfg.TitlePage {
		return false
	}
	content.WriteString("#bookset-running-heads.update(false)\n")
	if cfg.CoverPath != "" {
		fmt.Fprintf(content, "#align(center + horizon)[#image(\"%s\", width: 100%%)]\n", typstRawString(cfg.CoverPath))
		if cfg.TitlePage {
			content.WriteString("#pagebreak()\n")
		}
	}
	if cfg.TitlePage {
		fmt.Fprintf(content, "#align(center)[#v(2.1in)#text(font: %q, size: 25pt, weight: \"bold\")[%s]#v(1.1in)#text(font: %q, size: 12pt)[%s]]\n", cfg.HeadingFont, typstEscape(cfg.BookTitle), cfg.UtilityFont, typstEscape(cfg.BookAuthor))
	}
	return true
}

func writeBookmark(content *strings.Builder, docs []*markdown.Document, index int, doc *markdown.Document) {
	level := 1
	if doc.BookKind == "chapter" && hasIncludedPartBefore(docs, index) {
		level = 2
	}
	content.WriteString("#show heading.where(bookmarked: true): it => []\n")
	fmt.Fprintf(content, "#heading(level: %d, outlined: false, bookmarked: true)[%s]\n", level, typstEscape(chapterTitle(doc)))
}

func hasIncludedPartBefore(docs []*markdown.Document, index int) bool {
	for i := index - 1; i >= 0; i-- {
		if docs[i].BookKind == "part" {
			return !docs[i].ExcludeFromTOC
		}
	}
	return false
}

func writePrintSection(content *strings.Builder, section string, cfg style.Config) {
	if section == "front" {
		fmt.Fprintf(content, "#bookset-folios.update(%q)\n#bookset-running-heads.update(false)\n", cfg.FrontMatterFolios)
		return
	}
	if section == "main" {
		content.WriteString("#bookset-folios.update(\"arabic\")\n#bookset-running-heads.update(true)\n#counter(page).update(1)\n")
		return
	}
	content.WriteString("#bookset-folios.update(\"arabic\")\n#bookset-running-heads.update(true)\n")
}

func tocLabel(id string) string { return "bookset-toc-" + id }

func writeTOC(content *strings.Builder, docs []*markdown.Document, toc *markdown.Document, cfg style.Config) {
	fmt.Fprintf(content, "#align(center)[#text(font: %q, size: 18pt)[%s]]\n", cfg.HeadingFont, typstEscape(toc.Title))
	content.WriteString("#v(1.25em)\n")
	for _, entry := range tocEntries(docs) {
		doc := entry.doc
		title := chapterTitle(doc)
		if title == "" {
			continue
		}
		folio := fmt.Sprintf("#context counter(page).at(label(%q)).first()", tocLabel(doc.BookID))
		if doc.PrintSection == "front" && cfg.FrontMatterFolios == "roman" {
			folio = fmt.Sprintf("#context numbering(\"i\", counter(page).at(label(%q)).first())", tocLabel(doc.BookID))
		}
		if doc.PrintSection == "front" && cfg.FrontMatterFolios == "none" {
			folio = ""
		}
		indent := ""
		if entry.depth > 0 {
			indent = "#h(1.25em)"
		}
		fmt.Fprintf(content, "#block(above: .3em, below: .3em)[%s#link(label(%q))[%s] #h(1fr) %s]\n", indent, tocLabel(doc.BookID), typstEscape(title), folio)
	}
}

type tocEntry struct {
	doc   *markdown.Document
	depth int
}

func tocEntries(docs []*markdown.Document) []tocEntry {
	entries := make([]tocEntry, 0, len(docs))
	partActive := false
	for _, doc := range docs {
		if doc.BookKind == "part" {
			partActive = !doc.ExcludeFromTOC
		}
		if doc.ExcludeFromTOC || doc.BookKind == "toc" || doc.BookID == "" {
			continue
		}
		depth := 0
		if doc.BookKind == "chapter" && partActive {
			depth = 1
		}
		entries = append(entries, tocEntry{doc: doc, depth: depth})
	}
	return entries
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
	b.WriteString("#let bookset-folios = state(\"bookset-folios\", \"arabic\")\n#let bookset-running-heads = state(\"bookset-running-heads\", true)\n#let bookset-folio(value) = if bookset-folios.get() == \"roman\" { numbering(\"i\", value) } else { value }\n")
	fmt.Fprintf(&b, "#set par(justify: true, leading: %s, first-line-indent: 0.23in, spacing: 0.60em)\n", leading)
	fmt.Fprintf(&b, "#show heading.where(level: 1): it => block(above: 1.2em, below: .6em)[#align(center)[#text(font: %q, size: 16pt)[#it.body]]]\n", cfg.HeadingFont)
	fmt.Fprintf(&b, "#show heading.where(level: 2): it => block(above: .8em, below: .4em)[#text(font: %q, size: 11pt, weight: \"bold\")[#it.body]]\n", cfg.UtilityFont)
	if cfg.RunningHeads {
		bookTitle := cfg.BookTitle
		if bookTitle == "" {
			bookTitle = doc.Title
		}
		fmt.Fprintf(&b, "#let running-head(p) = { if calc.even(p) { grid(columns: (1fr, auto, 1fr), align: (left, center, right), [#bookset-folio(p)], [#text(size: 7.5pt, font: %q, weight: \"medium\", tracking: 0.08em)[#upper(%q)]], []) } else { grid(columns: (1fr, auto, 1fr), align: (left, center, right), [], [#text(size: 7.5pt, font: %q, style: \"italic\")[#bookset-chapter.get()]], [#bookset-folio(p)]) } }\n", cfg.UtilityFont, bookTitle, cfg.HeadingFont)
		b.WriteString("#set page(header: context { let p = counter(page).get().first(); if bookset-running-heads.get() and p > 1 { running-head(p) } })\n")
	} else {
		b.WriteString("#set page(header: none, footer: none)\n")
	}
	return b.String()
}

func Render(path string, doc *markdown.Document, cfg style.Config) error {
	return RenderDocuments(path, []*markdown.Document{doc}, cfg)
}

type RenderOptions struct {
	// SourcePath writes the exact generated Typst source used for compilation.
	// It is useful for diagnosing a Typst error outside bookset.
	SourcePath string
}

func RenderWithOptions(path string, doc *markdown.Document, cfg style.Config, options RenderOptions) error {
	return RenderDocumentsWithOptions(path, []*markdown.Document{doc}, cfg, options)
}

func RenderDocuments(path string, docs []*markdown.Document, cfg style.Config) error {
	return RenderDocumentsWithOptions(path, docs, cfg, RenderOptions{})
}

func RenderDocumentsWithOptions(path string, docs []*markdown.Document, cfg style.Config, options RenderOptions) error {
	_, err := renderDocuments(path, docs, cfg, options, false)
	return err
}

type ProofEntry struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Section   string `json:"section"`
	Title     string `json:"title"`
	StartPage int    `json:"start_page"`
	EndPage   int    `json:"end_page"`
	Folio     int    `json:"folio"`
}

// ProofDocuments renders a PDF and returns final-layout physical page spans.
func ProofDocuments(path string, docs []*markdown.Document, cfg style.Config) ([]ProofEntry, error) {
	return renderDocuments(path, docs, cfg, RenderOptions{}, true)
}

func renderDocuments(path string, docs []*markdown.Document, cfg style.Config, options RenderOptions, proof bool) ([]ProofEntry, error) {
	typst, err := exec.LookPath("typst")
	if err != nil {
		return nil, fmt.Errorf("typst is required for PDF rendering: %w", err)
	}
	if err := validateConfiguredFonts(typst, cfg); err != nil {
		return nil, err
	}
	if cfg.FontManifest != "" {
		if err := validateFonts(typst, cfg); err != nil {
			return nil, err
		}
	}
	tmp, err := os.MkdirTemp("", "bookset-typst-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	renderCfg, err := stageCover(tmp, cfg)
	if err != nil {
		return nil, err
	}
	sourceText, err := renderSource(docs, renderCfg)
	if err != nil {
		return nil, err
	}
	if options.SourcePath != "" {
		if err := os.MkdirAll(filepath.Dir(options.SourcePath), 0755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(options.SourcePath, []byte(sourceText), 0644); err != nil {
			return nil, err
		}
	}
	source := filepath.Join(tmp, "book.typ")
	if err := os.WriteFile(source, []byte(sourceText), 0644); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	args := []string{"compile"}
	if cfg.FontDir != "" {
		args = append(args, "--font-path", cfg.FontDir)
	}
	args = append(args, "--root", tmp, source, path)
	cmd := exec.Command(typst, args...)
	cmd.Env = append(os.Environ(), "SOURCE_DATE_EPOCH=0", "LANG="+cfg.Language+"_US.UTF-8", "LC_ALL="+cfg.Language+"_US.UTF-8")
	if output, err := cmd.CombinedOutput(); err != nil {
		diagnostic := string(bytes.TrimSpace(output))
		if sourceLocation := sourceLocationForTypstDiagnostic(sourceText, diagnostic); sourceLocation != "" {
			diagnostic += "\nbookset source: " + sourceLocation
		}
		if options.SourcePath != "" {
			return nil, fmt.Errorf("typst compile (source: %s): %w: %s", options.SourcePath, err, diagnostic)
		}
		return nil, fmt.Errorf("typst compile: %w: %s", err, diagnostic)
	}
	if !proof {
		return nil, nil
	}
	return queryProof(typst, tmp, source, renderCfg)
}

func queryProof(typstPath, root, source string, cfg style.Config) ([]ProofEntry, error) {
	args := []string{"eval"}
	if cfg.FontDir != "" {
		args = append(args, "--font-path", cfg.FontDir)
	}
	args = append(args, "--root", root, "--in", source, `query(<bookset-entry>)`)
	cmd := exec.Command(typstPath, args...)
	cmd.Env = append(os.Environ(), "SOURCE_DATE_EPOCH=0", "LANG="+cfg.Language+"_US.UTF-8", "LC_ALL="+cfg.Language+"_US.UTF-8")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("query PDF proof metadata: %w", err)
	}
	var records []struct {
		Value struct {
			ID      string `json:"id"`
			Kind    string `json:"kind"`
			Section string `json:"section"`
			Title   string `json:"title"`
			Page    int    `json:"page"`
			Folio   int    `json:"folio"`
		} `json:"value"`
	}
	if err := json.Unmarshal(output, &records); err != nil {
		return nil, fmt.Errorf("parse PDF proof metadata: %w", err)
	}
	entries := make([]ProofEntry, 0, len(records))
	for _, record := range records {
		entries = append(entries, ProofEntry{ID: record.Value.ID, Kind: record.Value.Kind, Section: record.Value.Section, Title: record.Value.Title, StartPage: record.Value.Page, Folio: record.Value.Folio})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("PDF proof metadata contains no manifest entries")
	}
	endArgs := []string{"eval"}
	if cfg.FontDir != "" {
		endArgs = append(endArgs, "--font-path", cfg.FontDir)
	}
	endArgs = append(endArgs, "--root", root, "--in", source, `query(<bookset-proof-end>)`)
	endCmd := exec.Command(typstPath, endArgs...)
	endCmd.Env = cmd.Env
	endOutput, err := endCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("query PDF proof end: %w", err)
	}
	var ends []struct {
		Value struct {
			Page int `json:"page"`
		} `json:"value"`
	}
	if err := json.Unmarshal(endOutput, &ends); err != nil || len(ends) != 1 {
		return nil, fmt.Errorf("parse PDF proof end metadata")
	}
	for i := range entries {
		entries[i].EndPage = ends[0].Value.Page
		if i+1 < len(entries) {
			entries[i].EndPage = entries[i+1].StartPage - 1
		}
	}
	return entries, nil
}

func stageCover(dir string, cfg style.Config) (style.Config, error) {
	if cfg.CoverPath == "" {
		return cfg, nil
	}
	ext := strings.ToLower(filepath.Ext(cfg.CoverPath))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" && ext != ".svg" {
		return cfg, fmt.Errorf("unsupported PDF cover format %q", ext)
	}
	data, err := os.ReadFile(cfg.CoverPath)
	if err != nil {
		return cfg, fmt.Errorf("read PDF cover image: %w", err)
	}
	staged := "cover" + ext
	if err := os.WriteFile(filepath.Join(dir, staged), data, 0644); err != nil {
		return cfg, err
	}
	cfg.CoverPath = staged
	return cfg, nil
}

var typstDiagnosticLocation = regexp.MustCompile(`(?m)book\.typ:(\d+):(\d+)`)
var sourceMarker = regexp.MustCompile(`^// bookset-source: (.+):(\d+)$`)

func sourceLocationForTypstDiagnostic(source, diagnostic string) string {
	match := typstDiagnosticLocation.FindStringSubmatch(diagnostic)
	if len(match) != 3 {
		return ""
	}
	line, err := strconv.Atoi(match[1])
	if err != nil || line < 1 {
		return ""
	}
	lines := strings.Split(source, "\n")
	if line > len(lines) {
		return ""
	}
	for index := line - 1; index >= 0; index-- {
		if marker := sourceMarker.FindStringSubmatch(lines[index]); len(marker) == 3 {
			return marker[1] + ":" + marker[2]
		}
	}
	return ""
}

func renderSource(docs []*markdown.Document, cfg style.Config) (string, error) {
	sourceText := SourceDocuments(docs, cfg)
	if cfg.TemplateDir == "" {
		return sourceText, nil
	}
	data, err := os.ReadFile(filepath.Join(cfg.TemplateDir, "chapter.typ"))
	if err == nil {
		return sourceDocumentsFromTemplate(docs, cfg, string(data)), nil
	}
	if cfg.TemplateRequired {
		return "", fmt.Errorf("configured Typst template unavailable: %w", err)
	}
	return sourceText, nil
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
	available, err := availableFonts(typstPath, cfg.FontDir)
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
	available, err := availableFonts(typstPath, cfg.FontDir)
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

// CheckConfiguredFonts verifies that Typst can resolve every font selected by
// cfg, including fonts supplied through cfg.FontDir. It performs no rendering.
func CheckConfiguredFonts(cfg style.Config) error {
	typstPath, err := exec.LookPath("typst")
	if err != nil {
		return fmt.Errorf("typst is required for PDF rendering: %w", err)
	}
	return validateConfiguredFonts(typstPath, cfg)
}

func availableFonts(typstPath, fontDir string) (map[string]bool, error) {
	output, err := exec.Command(typstPath, FontListArgs(fontDir)...).Output()
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

// FontListArgs returns the Typst command arguments used to list the same font
// search path that PDF rendering uses.
func FontListArgs(fontDir string) []string {
	args := []string{"fonts"}
	if fontDir != "" {
		args = append(args, "--font-path", fontDir)
	}
	return args
}

func requiredFontFamilies(cfg style.Config) map[string]bool {
	return map[string]bool{cfg.BodyFont: true, cfg.HeadingFont: true, cfg.UtilityFont: true}
}

const chapterTemplate = `{{.Setup}}
{{.Content}}
`

func writeBlocks(b *strings.Builder, blocks []semantic.Block, doc semantic.Document, cfg style.Config) {
	for _, block := range blocks {
		writeSourceMarker(b, doc.SourcePath, block.Source)
		writeSemanticBlock(b, block, doc, cfg)
	}
}

func writeSourceMarker(b *strings.Builder, path string, location markdown.SourceLocation) {
	if path == "" || location.Line == 0 {
		return
	}
	fmt.Fprintf(b, "// bookset-source: %s:%d\n", path, location.Line)
}

func writeSemanticBlock(b *strings.Builder, block semantic.Block, doc semantic.Document, cfg style.Config) {
	switch block.Kind {
	case semantic.ChapterOpener:
		fmt.Fprintf(b, "#bookset-chapter.update([%s])\n", typstEscape(markdown.PlainInline(block.Inlines)))
		fmt.Fprintf(b, "#chapter-title([%s], [%s])\n", inline(block.Inlines, doc), typstEscape(block.Label))
	case semantic.PartOpener:
		fmt.Fprintf(b, "#align(center)[#v(2in)#text(font: %q, size: 22pt, weight: \"bold\")[%s]]\n", cfg.HeadingFont, inline(block.Inlines, doc))
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
	writeBlockIndented(b, block, doc, cfg, "")
}

func writeBlockIndented(b *strings.Builder, block semantic.Block, doc semantic.Document, cfg style.Config, indent string) {
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
			writeBlockIndented(b, child, doc, cfg, indent)
		}
		b.WriteString("]\n")
	case semantic.List:
		for i, child := range block.Children {
			marker := "-"
			if block.Ordered {
				marker = fmt.Sprintf("%d.", block.Start+i)
			}
			fmt.Fprintf(b, "%s%s %s\n", indent, marker, inline(child.Inlines, doc))
			for _, nested := range child.Children {
				writeBlockIndented(b, nested, doc, cfg, indent+"  ")
			}
		}
	case semantic.ThematicBreak:
		b.WriteString("#align(center)[#text(size: 9pt)[• • •]]\n")
	}
}

func inline(inlines []markdown.Inline, doc semantic.Document) string {
	return inlineAtStart(inlines, doc)
}

var typstListMarker = regexp.MustCompile(`^\d+\.\s`)

func inlineAtStart(inlines []markdown.Inline, doc semantic.Document) string {
	var b strings.Builder
	atStart := true
	for _, in := range inlines {
		switch in.Kind {
		case markdown.Text:
			if atStart && typstListMarker.MatchString(in.Text) {
				// Typst recognizes "1. " as a list marker even inside an
				// emphasis content block. A zero-width inline element keeps the
				// marker literal without changing the printed result.
				b.WriteString("#h(0pt)")
			}
			b.WriteString(typstEscape(in.Text))
			if strings.TrimSpace(in.Text) != "" {
				atStart = false
			}
		case markdown.Emphasis:
			b.WriteString("#emph[")
			b.WriteString(inlineAtStart(in.Children, doc))
			b.WriteByte(']')
			atStart = false
		case markdown.Strong:
			b.WriteString("#strong[")
			b.WriteString(inlineAtStart(in.Children, doc))
			b.WriteByte(']')
			atStart = false
		case markdown.CodeSpan:
			b.WriteString(`#raw("`)
			b.WriteString(typstRawString(markdown.PlainInline(in.Children)))
			b.WriteString(`")`)
			atStart = false
		case markdown.Footnote:
			b.WriteString("#footnote[")
			b.WriteString(inlineAtStart(doc.Footnotes[in.Number], doc))
			b.WriteByte(']')
			atStart = false
		}
	}
	return b.String()
}

func typstEscape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	for _, char := range []string{"#", "[", "]", "{", "}", "*", "_", "$", "@", "<", ">", "`"} {
		value = strings.ReplaceAll(value, char, "\\"+char)
	}
	return value
}

func typstRawString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}
