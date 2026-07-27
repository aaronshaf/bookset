# Working in bookset

`bookset` is a deterministic publishing tool. Treat manuscript text and its
ordered manifest as user-owned source material: do not rewrite either merely
to work around a renderer, validator, or toolchain problem.

## Fast orientation

- The CLI lives in `cmd/bookset`.
- Markdown parsing and source validation are in `internal/markdown`.
- Complete-book loading and manifest rules are in `internal/book`.
- PDF and EPUB backends are `internal/typst` and `internal/epub`.
- `bookset.contents.example.toml` is the current complete-book example.
- `toolchain.toml` pins external publishing tools.

## Safe publishing workflow

Start with a read-only preflight:

```sh
go run ./cmd/bookset plan --config bookset.contents.example.toml --json
go run ./cmd/bookset doctor --config bookset.contents.example.toml
```

Build each deliverable independently, then inspect and validate it:

```sh
go run ./cmd/bookset build --config bookset.contents.example.toml --format pdf --output out/book.pdf
go run ./cmd/bookset proof --config bookset.contents.example.toml --output out/book.pdf --json
go run ./cmd/bookset inspect --config bookset.contents.example.toml --artifact out/book.pdf --json

go run ./cmd/bookset build --config bookset.contents.example.toml --format epub --output out/book.epub
go run ./cmd/bookset inspect --config bookset.contents.example.toml --artifact out/book.epub --json
make epubcheck EPUB=out/book.epub
```

Use `--typst-source out/book.typ` on PDF `build` or `render` when a Typst
failure needs diagnosis. Bookset maps generated Typst diagnostics back to the
nearest Markdown source line.

## Invariants agents should preserve

- `[[contents]]` order is the book's canonical reading order.
- Entry `id` values are stable identifiers used for PDF links, bookmarks, and
  proof reports. Do not derive identity from titles or filenames.
- Do not mix legacy `[[chapters]]` and typed `[[contents]]` in one manifest.
- Do not add a second `toc` entry or make synthetic PDF TOC content part of the
  EPUB spine.
- Front/main/back section order must not move backward.
- `book.cover_alt` is required whenever `book.cover` is set.
- Custom Typst templates are executable trusted input; do not render an
  unreviewed template in automation.

## Verification expectations

Run these after Go or renderer changes:

```sh
make check
go test -race ./...
```

When EPUB packaging changes, also build an EPUB and run `make epubcheck`.
When PDF layout changes, run a complete-book `proof` and inspect its page-span
report. Do not claim a PDF is validated if Typst, Poppler, or required fonts
were unavailable.

See [docs/agent-workflow.md](docs/agent-workflow.md) for the fuller publishing
workflow and [docs/markdown-format.md](docs/markdown-format.md) for the
manuscript contract.
