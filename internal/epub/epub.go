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
	contentNames := make([]string, len(docs))
	files := map[string][]byte{"mimetype": []byte("application/epub+zip"), "META-INF/container.xml": []byte(containerXML), "OEBPS/style.css": renderedStyles}
	for i, doc := range docs {
		name := "content.xhtml"
		if len(docs) > 1 {
			name = fmt.Sprintf("content-%03d.xhtml", i+1)
		}
		contentNames[i] = name
		files["OEBPS/"+name] = []byte(content(doc, cfg))
	}
	files["OEBPS/nav.xhtml"] = []byte(bookNav(docs, contentNames))
	files["OEBPS/package.opf"] = []byte(bookOPF(docs, contentNames))
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
		h := &zip.FileHeader{Name: name, Method: method, Modified: time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)}
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
const css = `body{font-family:"{{.BodyFont}}";font-size:{{.BodySize}};line-height:1.45}h1,h2,h3{font-family:"{{.HeadingFont}}"}.chapter-label,.then-now strong,.timeline-item strong{font-family:"{{.UtilityFont}}"}blockquote{margin:1em 2em;font-style:italic}li{margin:.3em 0}.footnotes{border-top:1px solid #999;margin-top:2em;font-size:.9em}`

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
			fmt.Fprintf(&b, `<sup><a href="#fn-%d" id="fnref-%d">%d</a></sup>`, v.Number, v.Number, v.Number)
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
		notes.WriteString(`<section class="footnotes"><h2>Notes</h2><ol>`)
		for n, note := range doc.Footnotes {
			fmt.Fprintf(&notes, `<li id="fn-%d">%s <a href="#fnref-%d">↩</a></li>`, n, inline(note, normalized), n)
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
	data := struct{ Language, Title, Body, Footnotes string }{esc(cfg.Language), esc(doc.Title), renderBlocks(normalized.Blocks, normalized), footnotes}
	if err := tmpl.Execute(&out, data); err != nil {
		return ""
	}
	return out.String()
}

const epubTemplate = `<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE html><html xmlns="http://www.w3.org/1999/xhtml" lang="{{.Language}}"><head><title>{{.Title}}</title><link rel="stylesheet" type="text/css" href="style.css"/></head><body>{{.Body}}{{.Footnotes}}</body></html>`

func bookNav(docs []*markdown.Document, names []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>Contents</title></head><body><nav epub:type="toc" id="toc"><h1>Contents</h1><ol>`)
	for i, doc := range docs {
		fmt.Fprintf(&b, `<li><a href="%s">%s</a></li>`, names[i], esc(doc.Title))
	}
	b.WriteString(`</ol></nav></body></html>`)
	return b.String()
}

func bookOPF(docs []*markdown.Document, names []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="book-id">%s</dc:identifier><dc:title>%s</dc:title><dc:creator>%s</dc:creator><dc:language>%s</dc:language></metadata><manifest>`, identifier(docs), esc(docs[0].Title), esc(docs[0].Author), esc(docs[0].Language))
	for i, name := range names {
		fmt.Fprintf(&b, `<item id="content-%03d" href="%s" media-type="application/xhtml+xml"/>`, i+1, name)
	}
	b.WriteString(`<item id="style" href="style.css" media-type="text/css"/><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/></manifest><spine>`)
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
