# Architecture

`bookset` keeps manuscript semantics independent from layout:

```text
Markdown + front matter
        │
        ▼
Goldmark AST → internal/markdown.Document → validation
                                      ├── internal/typst → Typst → PDF
                                      └── internal/epub  → EPUB 3
```

For complete books, `bookset build --config bookset.toml` loads an ordered
`[[chapters]]` manifest. PDF chapters are concatenated with deterministic
chapter breaks; EPUB chapters become separate XHTML documents with ordered
spine entries and navigation links.

Book builds validate the final artifact against every manifest chapter before
reporting success. The same check is available explicitly with
`bookset validate --config bookset.toml --artifact book.pdf` (or `.epub`).
`[book].chapter_numbering = true` appends each one-based manifest position to
`[book].chapter_label`; an individual `[[chapters]].chapter_label` is an
explicit override. This makes chapter identity book metadata rather than a
literal duplicated in every source document.

`bookset inspect --artifact FILE --json` emits the stable
`bookset.artifact-inspection.v1` report for agents and CI. It includes the
artifact digest, format-specific facts, ordered checks, and an overall
`ok`, `warning`, or `error` status. Use `--strict` when missing optional
inspection tools should fail a release.
The contract is checked in at
`docs/schemas/artifact-inspection.v1.json`; the render CI job publishes the
PDF and EPUB reports as workflow artifacts and asserts an `ok` status.
Adding `INPUT.md` performs source-to-artifact fidelity checks in the same
report, with stable issue codes such as `fidelity.text`, `fidelity.italic`,
`fidelity.bold`, and `fidelity.footnote`.
For complete books, replace `INPUT.md` with `--config bookset.toml`; the
report then includes ordered chapter paths and titles, and each fidelity issue
includes its one-based `chapter` number.
When used with `--config` for a PDF, the report also records the configured
font families actually needed by the normalized manuscript and fails when an
expected family is absent from `pdffonts` output. PDF subset prefixes and style
suffixes are normalized during this comparison.

The document model contains headings, paragraphs, quotes, lists, text,
emphasis, strong text, and footnote references/definitions. A renderer-level
semantic normalization pass recognizes project-configured structures such as
chapter openers, Then/Now pairs, timelines, and numbered sections. Unsupported
nodes are validation errors; they are never silently discarded. Plain-text
projection and footnote-set checks provide the first textual-fidelity guard.

Artifact validation is a second guard. PDF validation uses `pdftotext`,
`pdfinfo`, and `pdffonts`; EPUB validation checks the ZIP/XML container, XHTML
text, formatting tags, and footnote links. Standalone inspection can report a
missing tool as a warning. Configuration-aware PDF builds, validation, and
inspection instead fail without `pdffonts`, because they cannot prove the
selected font families were embedded.

Styles are selected by name or TOML path. Project TOML can override book
metadata, trim, typography, margins, pagination flags, and the template
directory. The Typst and EPUB backends load `templates/chapter.typ`,
`templates/epub.xhtml`, and `templates/styles.css`, with built-in fallbacks.
Templates receive the selected book title, chapter title, and typography values
(`BodyFont`, `HeadingFont`, `UtilityFont`, `BodySize`, and effective `Leading`)
so style presets remain configuration rather than hidden template constants.
Intentional project structures such as Then/Now openers or timelines belong in
the semantic/template layer and must not be inferred from chapter names.

Running heads are semantic template inputs as well: `[book].title` supplies the
book or series title, while the first H1 supplies the chapter title. Print
templates may mirror these across verso/recto pages and own folio placement;
automatic page-number footers should be disabled when doing so.

Footnote templates use a compact `par.leading` line-box gap and separate block
spacing for note separation. These values should be evaluated with a real
multi-line footnote render; `par.leading` is not a direct baseline-to-baseline
measurement.

Determinism comes from pinned Go modules, stable style values, fixed EPUB ZIP
timestamps and identifiers, fixed locale/language settings, and Typst's
reproducible compilation environment. Fonts are external inputs and must be
pinned by the release build; bundled presets use open font families documented
in `README.md`. CI enforces the pinned Typst version
and runs the representative semantic chapter through both output backends;
the exact CI toolchain is recorded in `toolchain.toml`.

For strict PDF releases, `[fonts].manifest` names a TOML lock file. Each
`[[font]]` entry records a Typst family, a font file path, and its SHA-256.
Every configured body, heading, and utility family must appear exactly once;
the PDF backend checks availability and checksums before invoking Typst.
