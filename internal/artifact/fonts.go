package artifact

import (
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/semantic"
	"github.com/aaronshaf/bookset/internal/style"
)

// expectedPDFFontFamilies returns only font families that the normalized
// manuscript causes the selected template to use. This avoids treating an
// unused configured role as a rendering failure while still checking every
// family that should have been embedded.
func expectedPDFFontFamilies(docs []*markdown.Document, cfg style.Config, pages int) []string {
	roles := map[string]bool{}
	for _, doc := range docs {
		for _, block := range semantic.Normalize(doc, cfg).Blocks {
			collectFontRoles(block, cfg, roles)
		}
	}
	if cfg.RunningHeads && pages > 1 {
		roles["utility"] = true
		roles["heading"] = true
	}
	fonts := make([]string, 0, len(roles))
	if roles["body"] && cfg.BodyFont != "" {
		fonts = append(fonts, cfg.BodyFont)
	}
	if roles["heading"] && cfg.HeadingFont != "" {
		fonts = append(fonts, cfg.HeadingFont)
	}
	if roles["utility"] && cfg.UtilityFont != "" {
		fonts = append(fonts, cfg.UtilityFont)
	}
	return uniqueSorted(fonts)
}

func collectFontRoles(block semantic.Block, cfg style.Config, roles map[string]bool) {
	switch block.Kind {
	case semantic.Paragraph, semantic.Quote, semantic.List:
		roles["body"] = true
	case semantic.Heading, semantic.Section:
		roles["heading"] = true
		if block.Kind == semantic.Heading && block.Level >= 2 && filepath.Base(cfg.TemplateDir) != "timeline-trade" {
			roles["utility"] = true
		}
	case semantic.ChapterOpener:
		roles["heading"] = true
		roles["utility"] = true
	case semantic.ThenNow, semantic.Timeline:
		roles["body"] = true
		roles["utility"] = true
	}
	for _, child := range block.Children {
		collectFontRoles(child, cfg, roles)
	}
}

func missingExpectedFontFamilies(expected, embedded []string) []string {
	var missing []string
	for _, family := range expected {
		found := false
		for _, actual := range embedded {
			if embeddedFontMatches(actual, family) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, family)
		}
	}
	return missing
}

func embeddedFontMatches(actual, expected string) bool {
	actual = canonicalFontFamily(actual)
	expected = canonicalFontFamily(expected)
	return actual == expected || strings.HasPrefix(actual, expected)
}

func canonicalFontFamily(value string) string {
	if _, suffix, ok := strings.Cut(value, "+"); ok {
		value = suffix // PDF subset tags are e.g. ABCDEF+SourceSerif4-Regular.
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	for _, value := range values {
		if value != "" {
			seen[value] = true
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
