package markdown

// Document is the small, renderer-independent manuscript model used by all
// bookset backends.
type Document struct {
	SourcePath     string
	BookID         string
	BookKind       string
	PrintSection   string
	ExcludeFromTOC bool
	Title          string
	Author         string
	Language       string
	ChapterLabel   string
	Blocks         []Block
	Footnotes      map[int][]Inline
	sourceOffset   int
}

// SourceLocation identifies the source line that produced a model node.
// Column is reserved for a future finer-grained inline diagnostic map.
type SourceLocation struct {
	Line   int
	Column int
}

type Block struct {
	Source   SourceLocation
	Kind     BlockKind
	Level    int
	Inlines  []Inline
	Children []Block
	Ordered  bool
	Start    int
}

type BlockKind string

const (
	Heading       BlockKind = "heading"
	Paragraph     BlockKind = "paragraph"
	Quote         BlockKind = "quote"
	List          BlockKind = "list"
	ListItem      BlockKind = "list-item"
	ThematicBreak BlockKind = "thematic-break"
)

type Inline struct {
	Kind     InlineKind
	Text     string
	Children []Inline
	Number   int
}

type InlineKind string

const (
	Text     InlineKind = "text"
	Emphasis InlineKind = "emphasis"
	Strong   InlineKind = "strong"
	CodeSpan InlineKind = "code-span"
	Footnote InlineKind = "footnote"
)

func (d *Document) PlainText() string {
	var out string
	for _, block := range d.Blocks {
		out += blockText(block)
	}
	return out
}

func blockText(block Block) string {
	if block.Kind == List || block.Kind == Quote {
		var out string
		for _, child := range block.Children {
			out += blockText(child)
		}
		return out
	}
	if block.Kind == ListItem {
		out := inlineText(block.Inlines) + "\n"
		for _, child := range block.Children {
			out += blockText(child)
		}
		return out
	}
	return inlineText(block.Inlines) + "\n"
}

func inlineText(inlines []Inline) string {
	var out string
	for _, inline := range inlines {
		switch inline.Kind {
		case Text:
			out += inline.Text
		case Emphasis, Strong, CodeSpan:
			out += inlineText(inline.Children)
		case Footnote:
			// A reference is represented by its semantic number, not its
			// Markdown delimiters. Definitions are checked separately.
		}
	}
	return out
}

// PlainInline returns the manuscript text represented by inline nodes,
// omitting formatting markers and footnote references.
func PlainInline(inlines []Inline) string { return inlineText(inlines) }
