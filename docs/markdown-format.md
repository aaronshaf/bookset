# Markdown format

`bookset` accepts UTF-8 Markdown with optional YAML-like front matter. It is a
deliberately small publishing format rather than a renderer for every
CommonMark extension.

## Front matter

Front matter must be the first content in a source file and use this form:

```markdown
---
title: "Example Book"
author: "Example Author"
language: "en"
---
```

`title`, `author`, and `language` are optional. If `title` is omitted,
`bookset` uses the first level-one heading. The parser accepts simple
single-line `key: value` pairs; it does not implement full YAML.

## Supported Markdown

- headings;
- paragraphs and block quotes;
- thematic breaks (`---`, rendered as a centered ornament in PDF and a rule in EPUB);
- ordered and unordered lists with inline list-item content;
- emphasis, strong emphasis, nested emphasis, and inline code;
- footnote references and definitions;
- literal angle-bracket transcription such as `\<Moroni>` and character
  entities such as `&lt;Moroni&gt;`. Raw HTML is treated as literal text rather
  than executable markup.

Unsupported block or inline constructs are validation errors. This is
intentional: a publishing workflow should reject content it cannot preserve
rather than silently omit it.

## Semantic structures

The `timeline-trade` preset recognizes a few explicit structures after Markdown
parsing:

- `**Then:** ...` and `**Now:** ...` paragraphs;
- a level-two `Timeline` heading followed by list items whose bold label is a
  date;
- chapter openers and unnumbered sections configured by the selected style.

Other presets retain ordinary Markdown headings and paragraphs.

## Trust boundary

Manuscripts and styles are local publishing inputs. Custom Typst templates are
trusted code: rendering one evaluates its Typst source. Review templates and
font manifests before using them in an automated publishing environment.
