# bookset

Deterministic book rendering from Markdown.

`bookset` is intended to turn a structured Markdown manuscript into polished
print and ebook editions while preserving the source text and its semantic
formatting. The project is designed around one parsed document model with
multiple output backends:

```text
Markdown → AST → Typst/PDF
              └→ EPUB/XHTML
```

## Status

The current implementation parses a supported Markdown subset with Goldmark,
validates EPUB 3 output as a ZIP container, and compiles PDF output through
Typst. The model and validation layer are deliberately small so subject-
specific templates can be added without changing manuscript semantics.

## Installation

Install the command-line tool with Go:

```sh
go install github.com/aaronshaf/bookset/cmd/bookset@latest
```

To work from a checkout, use `go run ./cmd/bookset ...` or build it with
`make build`.

## Requirements

- Go 1.25 or newer;
- Typst 0.15.1 for PDF rendering;
- Poppler utilities (`pdfinfo`, `pdffonts`, and `pdftotext`) for complete PDF
  inspection and validation;
- fonts required by the selected preset.

All bundled presets use Source Serif 4 and Source Sans 3. Font files are
external inputs and are not distributed by this repository.

```sh
go run ./cmd/bookset version
go run ./cmd/bookset render --format pdf --output out/book.pdf testdata/field-notes.md
go run ./cmd/bookset render --format epub --output out/book.epub testdata/field-notes.md
go run ./cmd/bookset validate testdata/field-notes.md
go run ./cmd/bookset validate --artifact out/book.pdf testdata/field-notes.md
go run ./cmd/bookset inspect testdata/field-notes.md
go run ./cmd/bookset inspect --artifact out/book.pdf --json
go run ./cmd/bookset inspect --artifact out/book.pdf --json testdata/field-notes.md
go run ./cmd/bookset build --config bookset.book.example.toml \
  --format epub --output out/book.epub
go run ./cmd/bookset inspect --config bookset.book.example.toml \
  --artifact out/book.epub --json
go run ./cmd/bookset validate --config bookset.book.example.toml \
  --artifact out/book.epub

# Project and file-based style configuration
go run ./cmd/bookset render --config bookset.example.toml \
  --style styles/trade.toml --format pdf --output out/book.pdf testdata/field-notes.md

# US Letter proof sheet around a 6×9 trim, with crop marks
go run ./cmd/bookset render --style timeline-trade --format pdf \
  --sheet letter --trim-marks --output out/book-proof.pdf testdata/field-notes.md
```

Use `--style trade` for the neutral fixture, `--style classic-trade` for the
generic 6×9 trade-book preset, or `--style timeline-trade` with the example
book config for the semantic chapter opener and timeline treatment. Styles are
data-only configurations; semantic structures are rendered by project templates.
The optional `--sheet letter --trim-marks` mode keeps the configured 6×9 text
block
but emits an 8.5×11 proof sheet with crop marks.

PDF rendering requires Typst 0.15.1 (or a compatible pinned version) on
`PATH`. Before rendering, bookset verifies that every configured body,
heading, and utility family is available to Typst. Configuration-aware PDF
validation and inspection then verify the families embedded in the result, so
silent font substitution becomes a failure. `go-toml/v2` is used because Go
has no standard TOML package; Goldmark provides the mature CommonMark/GFM AST
and footnote parser.
For reproducible releases, pin the Typst binary and record font file
checksums in the build environment. A project can enforce this before PDF
rendering with `[fonts].manifest`, pointing to a lock file containing
`[[font]]` entries with `family`, `path`, and `sha256` fields; every configured
body, heading, and utility family must appear exactly once. The pinned CI
toolchain is documented in [toolchain.toml](toolchain.toml). CI runs the
semantic publishing gate with Typst and Poppler installed; local tests still
skip PDF smoke checks when those optional tools are unavailable. A PDF build,
validation, or inspection made with `--config` requires Poppler's `pdffonts`
to verify configured font families.

For complete books, set `chapter_label = "CHAPTER"` and
`chapter_numbering = true` in `[book]` to produce `CHAPTER 1`, `CHAPTER 2`,
and so on. A `chapter_label` in an individual `[[chapters]]` entry takes
precedence, which is useful for an interlude or a deliberately named chapter.

Custom template directories are trusted publishing inputs. Rendering a custom
Typst template evaluates its Typst source, so do not render templates obtained
from an untrusted party.
The `github.com/santhosh-tekuri/jsonschema/v6` dependency validates the
versioned machine-readable inspection contract in tests and CI.

## Design goals

- deterministic output from pinned tools, fonts, and style configuration;
- high textual fidelity for emphasis, footnotes, quotations, lists, and
  escaped punctuation;
- print-ready PDF through Typst;
- reflowable EPUB with accessible XHTML and navigation;
- fixture-driven tests using synthetic, non-controversial sample material;
- a clean separation between manuscript semantics and visual styles;
- a deterministic path from ordered chapter manifests to complete books.

## Packages

- `internal/markdown` — Markdown parsing and normalized document model;
- `internal/typst` — print layout, semantic pagination, and PDF backend;
- `internal/epub` — EPUB container, XHTML, CSS, and navigation backend;
- `internal/markdown` validation — source/model fidelity and footnote checks;
- `internal/style` — versioned, selectable style presets;
- `internal/config` — project and book-manifest configuration;
- `internal/artifact` — rendered-artifact inspection and fidelity validation.

## Local checks

Run the complete local quality gate with:

```sh
make check
```

Run dependency analysis and static checks with `make security-check`, and run
the parser and artifact fuzz targets with `make fuzz`.

The release workflow is currently manual-only. When enabled, it accepts an
existing `v*` tag, cross-compiles macOS, Linux, and Windows binaries, stamps
the tag into `bookset version`, and attaches SHA-256 checksums to the GitHub
release.

PDF-specific checks may be skipped when Typst or Poppler is unavailable. The
continuous-integration workflow installs those tools and runs the complete
publishing gate.

See [docs/architecture.md](docs/architecture.md) for the pipeline and
template boundary. See [docs/markdown-format.md](docs/markdown-format.md) for
the supported Markdown and front-matter contract.

## License

MIT. See [LICENSE](LICENSE).
