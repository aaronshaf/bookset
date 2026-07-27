# Agent publishing workflow

This is the canonical operating sequence for an agent working with a Bookset
manuscript. It keeps source review, rendering, and artifact verification
separate so a failed tool never becomes a reason to change authored text.

## 1. Read the manifest, do not infer order

The ordered `[[contents]]` list is the book. Use the plan command before any
rendering:

```sh
bookset plan --config bookset.toml --json
```

The report contains stable entry IDs, kinds, print sections, assigned chapter
numbers, source paths, TOC eligibility, and body-word counts. It fails on
duplicate source files and source-backed entries that have no body text.

Do not scan a directory and silently append Markdown files. If a source is
missing from the manifest, report that decision to the manuscript owner.

## 2. Check external dependencies

Before PDF work, run:

```sh
bookset doctor --config bookset.toml
```

PDF rendering needs the pinned Typst version, selected fonts, and Poppler for
full inspection. EPUB packaging additionally has an optional Java-based
EPUBCheck gate. Missing dependencies are environment problems, not manuscript
problems.

## 3. Build one format at a time

```sh
bookset build --config bookset.toml --format pdf --output out/book.pdf
bookset build --config bookset.toml --format epub --output out/book.epub
```

For a Typst failure, retry PDF generation with `--typst-source out/book.typ`.
The generated source contains Markdown location markers; preserve the failed
source and error output while diagnosing the renderer.

## 4. Verify the final artifacts

```sh
bookset proof --config bookset.toml --output out/book.pdf --json
bookset inspect --config bookset.toml --artifact out/book.pdf --json
bookset inspect --config bookset.toml --artifact out/book.epub --json
make epubcheck EPUB=out/book.epub
```

`proof` reports physical PDF page spans and displayed folios from Typst's final
layout. `inspect --json` exposes the stable artifact-inspection contract for
automation. Treat `error` as a release blocker; decide explicitly whether a
`warning` is acceptable.

## 5. Change the correct layer

| Problem | Correct layer |
| --- | --- |
| Unsupported Markdown or malformed front matter | manuscript/source validation |
| Wrong chapter order or numbering | manifest |
| Missing font, Typst, Poppler, or Java | environment/toolchain |
| Bad page break, header, or PDF folio | Typst/style/template |
| EPUB package or navigation issue | EPUB backend/package metadata |

Never flatten semantic Markdown, remove footnotes, or edit historical text to
make a renderer pass. Add a regression first when a renderer loses or corrupts
valid content.

## Machine-readable interfaces

- `bookset plan --json` — resolved book sequence;
- `bookset proof --json` — final PDF page spans;
- `bookset inspect --json` — artifact checks and fidelity issues.

These commands are intended for automation. Prefer them over scraping human
command output.
