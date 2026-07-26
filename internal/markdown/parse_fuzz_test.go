package markdown

import "testing"

func FuzzParse(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("# Title\n\nText.\n"),
		[]byte("---\ntitle: Example\nauthor: Author\nlanguage: en\n---\n\n# Title\n\nText[^1].\n\n[^1]: Note.\n"),
		[]byte("---\ntitle: Unclosed\n\n# Title\n"),
		[]byte("# Title\n\n`code` and ***nested emphasis***.\n"),
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		doc, issues := Parse(source)
		if doc == nil {
			t.Fatal("Parse returned a nil document")
		}
		_ = Validate(doc, issues)
		_ = doc.PlainText()
	})
}
