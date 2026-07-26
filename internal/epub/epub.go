package epub

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/semantic"
	"github.com/aaronshaf/bookset/internal/style"
)

func Write(path string, doc *markdown.Document, cfg style.Config) error {
	return WriteBook(path, []*markdown.Document{doc}, cfg)
}

func WriteBook(path string, docs []*markdown.Document, cfg style.Config) error {
	docs = spineDocuments(docs)
	if len(docs) == 0 {
		return fmt.Errorf("cannot write an EPUB with no chapters")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	stylesheet := []byte(css)
	if cfg.TemplateDir != "" {
		if data, readErr := os.ReadFile(filepath.Join(cfg.TemplateDir, "styles.css")); readErr == nil {
			stylesheet = data
		} else if cfg.TemplateRequired {
			if _, chapterErr := os.Stat(filepath.Join(cfg.TemplateDir, "chapter.typ")); chapterErr != nil {
				return chapterErr
			}
		}
	}
	renderedStyles, err := renderStyles(string(stylesheet), cfg)
	if err != nil {
		return fmt.Errorf("render EPUB stylesheet: %w", err)
	}
	cover, err := loadCover(cfg)
	if err != nil {
		return err
	}
	contentNames := make([]string, len(docs))
	files := map[string][]byte{"mimetype": []byte("application/epub+zip"), "META-INF/container.xml": []byte(containerXML), "OEBPS/style.css": renderedStyles}
	if cover != nil {
		files["OEBPS/"+cover.name] = cover.data
		files["OEBPS/cover.xhtml"] = []byte(coverContent(cfg.Language, cover))
	}
	for i, doc := range docs {
		name := "content.xhtml"
		if len(docs) > 1 {
			name = fmt.Sprintf("content-%03d.xhtml", i+1)
		}
		contentNames[i] = name
		files["OEBPS/"+name] = []byte(content(doc, cfg))
	}
	files["OEBPS/nav.xhtml"] = []byte(bookNav(docs, contentNames, cover != nil))
	files["OEBPS/package.opf"] = []byte(bookOPF(docs, contentNames, cfg, cover))
	keys := make([]string, 0, len(files))
	for key := range files {
		if key != "mimetype" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	write := func(name string, data []byte, method uint16) error {
		h := &zip.FileHeader{Name: name, Method: method}
		if name != "mimetype" {
			h.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := write("mimetype", files["mimetype"], zip.Store); err != nil {
		return err
	}
	for _, key := range keys {
		if err := write(key, files[key], zip.Deflate); err != nil {
			return err
		}
	}
	return zw.Close()
}

type coverAsset struct {
	name, mediaType, alt string
	data                 []byte
}

func loadCover(cfg style.Config) (*coverAsset, error) {
	if cfg.CoverPath == "" {
		return nil, nil
	}
	ext := strings.ToLower(filepath.Ext(cfg.CoverPath))
	mediaTypes := map[string]string{".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".svg": "image/svg+xml"}
	mediaType := mediaTypes[ext]
	if mediaType == "" {
		return nil, fmt.Errorf("unsupported cover format %q; use JPEG, PNG, WebP, or SVG", ext)
	}
	data, err := os.ReadFile(filepath.Clean(cfg.CoverPath))
	if err != nil {
		return nil, fmt.Errorf("read cover image: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("cover image is empty")
	}
	return &coverAsset{name: "cover" + ext, mediaType: mediaType, alt: cfg.CoverAlt, data: data}, nil
}

func coverContent(language string, cover *coverAsset) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="%s" xml:lang="%s"><head><title>Cover</title><link rel="stylesheet" type="text/css" href="style.css"/></head><body epub:type="cover"><section epub:type="cover"><img src="%s" alt="%s"/></section></body></html>`, esc(language), esc(language), esc(cover.name), esc(cover.alt))
}

// spineDocuments omits the print-only TOC document. EPUB navigation is
// generated separately in nav.xhtml, so including it in the spine would
// create a duplicate, non-native contents page.
func spineDocuments(docs []*markdown.Document) []*markdown.Document {
	spine := make([]*markdown.Document, 0, len(docs))
	for _, doc := range docs {
		if doc.BookKind != "toc" {
			spine = append(spine, doc)
		}
	}
	return spine
}

func Validate(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	seen := map[string]bool{}
	for _, file := range r.File {
		seen[file.Name] = true
	}
	for _, required := range []string{"mimetype", "META-INF/container.xml", "OEBPS/style.css", "OEBPS/package.opf", "OEBPS/nav.xhtml"} {
		if !seen[required] {
			return fmt.Errorf("EPUB missing %s", required)
		}
	}
	contentCount := 0
	for name := range seen {
		if strings.HasPrefix(name, "OEBPS/content") && strings.HasSuffix(name, ".xhtml") {
			contentCount++
		}
	}
	if contentCount == 0 {
		return fmt.Errorf("EPUB contains no chapter XHTML documents")
	}
	for _, file := range r.File {
		if file.Name == "mimetype" {
			reader, err := file.Open()
			if err != nil {
				return err
			}
			b, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				return err
			}
			if string(b) != "application/epub+zip" {
				return fmt.Errorf("invalid EPUB mimetype")
			}
		}
	}
	xmlNames := []string{"META-INF/container.xml", "OEBPS/nav.xhtml", "OEBPS/package.opf"}
	for name := range seen {
		if strings.HasPrefix(name, "OEBPS/content") && strings.HasSuffix(name, ".xhtml") {
			xmlNames = append(xmlNames, name)
		}
	}
	for _, name := range xmlNames {
		file, err := r.Open(name)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			return err
		}
		if err := xml.Unmarshal(data, new(struct{})); err != nil {
			return fmt.Errorf("invalid XML in %s: %w", name, err)
		}
	}
	return nil
}

const containerXML = `<?xml version="1.0" encoding="UTF-8"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/package.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`
const css = `body{font-family:"{{.BodyFont}}";font-size:{{.BodySize}};line-height:1.45}h1,h2,h3{font-family:"{{.HeadingFont}}"}.chapter-label,.then-now strong,.timeline-item strong{font-family:"{{.UtilityFont}}"}blockquote{margin:1em 2em;font-style:italic}li{margin:.3em 0}.footnotes{border-top:1px solid #999;margin-top:2em;font-size:.9em}body[epub|type~="cover"]{margin:0;padding:0;text-align:center}section[epub|type~="cover"] img{max-width:100%;max-height:100vh}`

type styleTemplateData struct {
	BodyFont, HeadingFont, UtilityFont, BodySize string
}

func renderStyles(source string, cfg style.Config) ([]byte, error) {
	tmpl, err := template.New("styles").Parse(source)
	if err != nil {
		return nil, err
	}
	bodySize := cfg.BodySize
	if bodySize == "" {
		bodySize = "1em"
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, styleTemplateData{cfg.BodyFont, cfg.HeadingFont, cfg.UtilityFont, bodySize}); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

func esc(s string) string { return html.EscapeString(s) }
func inline(in []markdown.Inline, doc semantic.Document) string {
	var b strings.Builder
	for _, v := range in {
		switch v.Kind {
		case markdown.Text:
			b.WriteString(esc(v.Text))
		case markdown.Emphasis:
			b.WriteString("<em>" + inline(v.Children, doc) + "</em>")
		case markdown.Strong:
			b.WriteString("<strong>" + inline(v.Children, doc) + "</strong>")
		case markdown.CodeSpan:
			b.WriteString("<code>" + inline(v.Children, doc) + "</code>")
		case markdown.Footnote:
			fmt.Fprintf(&b, `<sup><a epub:type="noteref" href="#fn-%d" id="fnref-%d">%d</a></sup>`, v.Number, v.Number, v.Number)
		}
	}
	return b.String()
}
func renderBlocks(blocks []semantic.Block, doc semantic.Document) string {
	var b strings.Builder
	for _, v := range blocks {
		switch v.Kind {
		case semantic.Heading:
			fmt.Fprintf(&b, "<h%d>%s</h%d>", v.Level, inline(v.Inlines, doc), v.Level)
		case semantic.ChapterOpener:
			if v.Label != "" {
				b.WriteString(`<p class="chapter-label">` + esc(v.Label) + `</p>`)
			}
			b.WriteString("<h1>" + inline(v.Inlines, doc) + "</h1>")
		case semantic.PartOpener:
			b.WriteString(`<section class="part"><h1>` + inline(v.Inlines, doc) + `</h1></section>`)
		case semantic.Section:
			fmt.Fprintf(&b, "<h2>%d. %s</h2>", v.Number, inline(v.Inlines, doc))
		case semantic.ThenNow:
			b.WriteString(`<p class="then-now"><strong>` + esc(v.Label) + `</strong> ` + inline(v.Inlines, doc) + `</p>`)
		case semantic.Timeline:
			b.WriteString(`<section class="timeline"><h2>Timeline</h2>`)
			for _, item := range v.Children {
				b.WriteString(`<p class="timeline-item"><strong>` + esc(item.Date) + `</strong> ` + inline(item.Inlines, doc) + `</p>`)
			}
			b.WriteString(`</section>`)
		case semantic.Paragraph:
			b.WriteString("<p>" + inline(v.Inlines, doc) + "</p>")
		case semantic.Quote:
			b.WriteString("<blockquote>" + renderBlocks(v.Children, doc) + "</blockquote>")
		case semantic.List:
			tag := "ul"
			if v.Ordered {
				tag = "ol"
			}
			fmt.Fprintf(&b, "<%s>", tag)
			for _, item := range v.Children {
				b.WriteString("<li>" + inline(item.Inlines, doc) + renderBlocks(item.Children, doc) + "</li>")
			}
			fmt.Fprintf(&b, "</%s>", tag)
		case semantic.ThematicBreak:
			b.WriteString("<hr/>")
		}
	}
	return b.String()
}
func content(doc *markdown.Document, cfg style.Config) string {
	normalized := semantic.Normalize(doc, cfg)
	footnotes := ""
	if len(doc.Footnotes) > 0 {
		var notes strings.Builder
		notes.WriteString(`<section class="footnotes" epub:type="footnotes"><h2>Notes</h2><ol>`)
		for n, note := range doc.Footnotes {
			fmt.Fprintf(&notes, `<li id="fn-%d" epub:type="footnote">%s <a href="#fnref-%d">↩</a></li>`, n, inline(note, normalized), n)
		}
		notes.WriteString(`</ol></section>`)
		footnotes = notes.String()
	}
	templateText := epubTemplate
	if cfg.TemplateDir != "" {
		if data, readErr := os.ReadFile(filepath.Join(cfg.TemplateDir, "epub.xhtml")); readErr == nil {
			templateText = string(data)
		}
	}
	tmpl, err := template.New("epub").Parse(templateText)
	if err != nil {
		return ""
	}
	var out strings.Builder
	data := struct{ Language, Title, BodyType, Body, Footnotes string }{esc(cfg.Language), esc(doc.Title), bodyType(doc), renderBlocks(normalized.Blocks, normalized), footnotes}
	if err := tmpl.Execute(&out, data); err != nil {
		return ""
	}
	return out.String()
}

const epubTemplate = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops" lang="{{.Language}}" xml:lang="{{.Language}}"><head><title>{{.Title}}</title><link rel="stylesheet" type="text/css" href="style.css"/></head><body epub:type="{{.BodyType}}">{{.Body}}{{.Footnotes}}</body></html>`

func bodyType(doc *markdown.Document) string {
	switch doc.PrintSection {
	case "front":
		return "frontmatter"
	case "back":
		return "backmatter"
	default:
		return "bodymatter"
	}
}

func bookNav(docs []*markdown.Document, names []string, hasCover bool) string {
	nodes := navigationNodes(docs, names)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head><body><nav epub:type="toc" id="toc"><h1>Contents</h1><ol>`)
	writeNavNodes(&b, nodes)
	b.WriteString(`</ol></nav>`)
	if landmarks := landmarkNodes(docs, names, hasCover); len(landmarks) > 0 {
		b.WriteString(`<nav epub:type="landmarks" hidden=""><h2>Landmarks</h2><ol>`)
		for _, landmark := range landmarks {
			fmt.Fprintf(&b, `<li><a epub:type="%s" href="%s">%s</a></li>`, landmark.kind, landmark.href, landmark.title)
		}
		b.WriteString(`</ol></nav>`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

type navNode struct {
	title, href string
	children    []navNode
}

func navigationNodes(docs []*markdown.Document, names []string) []navNode {
	nodes := make([]navNode, 0, len(docs))
	partIndex := -1
	for i, doc := range docs {
		if doc.BookKind == "part" {
			partIndex = -1
		}
		if doc.ExcludeFromTOC {
			continue
		}
		node := navNode{title: doc.Title, href: names[i]}
		if doc.BookKind == "chapter" && partIndex >= 0 {
			nodes[partIndex].children = append(nodes[partIndex].children, node)
			continue
		}
		nodes = append(nodes, node)
		if doc.BookKind == "part" {
			partIndex = len(nodes) - 1
		}
	}
	return nodes
}

func writeNavNodes(b *strings.Builder, nodes []navNode) {
	for _, node := range nodes {
		fmt.Fprintf(b, `<li><a href="%s">%s</a>`, node.href, esc(node.title))
		if len(node.children) > 0 {
			b.WriteString(`<ol>`)
			writeNavNodes(b, node.children)
			b.WriteString(`</ol>`)
		}
		b.WriteString(`</li>`)
	}
}

type landmarkNode struct{ kind, href, title string }

func landmarkNodes(docs []*markdown.Document, names []string, hasCover bool) []landmarkNode {
	seen := map[string]bool{}
	landmarks := make([]landmarkNode, 0, 3)
	if hasCover {
		landmarks = append(landmarks, landmarkNode{kind: "cover", href: "cover.xhtml", title: "Cover"})
	}
	for i, doc := range docs {
		kind, title := "", ""
		switch doc.PrintSection {
		case "front":
			kind, title = "frontmatter", "Front Matter"
		case "main":
			kind, title = "bodymatter", "Main Matter"
		case "back":
			kind, title = "backmatter", "Back Matter"
		}
		if kind != "" && !seen[kind] {
			landmarks = append(landmarks, landmarkNode{kind: kind, href: names[i], title: title})
			seen[kind] = true
		}
	}
	return landmarks
}

func bookOPF(docs []*markdown.Document, names []string, cfg style.Config, cover *coverAsset) string {
	var b strings.Builder
	title := cfg.BookTitle
	if title == "" {
		title = docs[0].Title
	}
	author := cfg.BookAuthor
	if author == "" {
		author = docs[0].Author
	}
	modified := cfg.BookModified
	if modified == "" {
		modified = "1970-01-01T00:00:00Z"
	}
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="book-id">%s</dc:identifier><dc:title>%s</dc:title>`, identifier(docs), esc(title))
	if author != "" {
		fmt.Fprintf(&b, `<dc:creator>%s</dc:creator>`, esc(author))
	}
	fmt.Fprintf(&b, `<dc:language>%s</dc:language><meta property="dcterms:modified">%s</meta><meta property="schema:accessMode">textual</meta><meta property="schema:accessibilityFeature">structuralNavigation</meta><meta property="schema:accessibilityFeature">tableOfContents</meta></metadata><manifest>`, esc(docs[0].Language), esc(modified))
	if cover != nil {
		fmt.Fprintf(&b, `<item id="cover" href="cover.xhtml" media-type="application/xhtml+xml"/><item id="cover-image" href="%s" media-type="%s" properties="cover-image"/>`, cover.name, cover.mediaType)
	}
	for i, name := range names {
		fmt.Fprintf(&b, `<item id="content-%03d" href="%s" media-type="application/xhtml+xml"/>`, i+1, name)
	}
	b.WriteString(`<item id="style" href="style.css" media-type="text/css"/><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/></manifest><spine>`)
	if cover != nil {
		b.WriteString(`<itemref idref="cover"/>`)
	}
	for i := range names {
		fmt.Fprintf(&b, `<itemref idref="content-%03d"/>`, i+1)
	}
	b.WriteString(`</spine></package>`)
	return b.String()
}

func identifier(docs []*markdown.Document) string {
	hash := sha256.New()
	write := func(value string) {
		fmt.Fprintf(hash, "%d:%s\n", len(value), value)
	}
	fmt.Fprintf(hash, "chapters:%d\n", len(docs))
	for chapter, doc := range docs {
		fmt.Fprintf(hash, "chapter:%d\n", chapter+1)
		write(doc.Title)
		write(doc.Author)
		write(doc.Language)
		write(doc.PlainText())
		numbers := make([]int, 0, len(doc.Footnotes))
		for number := range doc.Footnotes {
			numbers = append(numbers, number)
		}
		sort.Ints(numbers)
		for _, number := range numbers {
			fmt.Fprintf(hash, "%d:", number)
			write(markdown.PlainInline(doc.Footnotes[number]))
		}
	}
	return "urn:sha256:" + hex.EncodeToString(hash.Sum(nil))
}
