package artifact

import "testing"

func FuzzPDFMetadataParsing(f *testing.F) {
	f.Add("Pages: 1\nPage size: 432 x 648 pts\n")
	f.Add("Pages: unknown\n")
	f.Fuzz(func(t *testing.T, info string) {
		_ = parseInfoValue(info, "Pages")
		_ = parseInfoInt(info, "Pages")
		_ = normalizePageSize(parseInfoValue(info, "Page size"))
		_ = parsePDFFonts(info)
	})
}
