package semantic

import (
	"testing"

	"github.com/aaronshaf/bookset/internal/markdown"
	"github.com/aaronshaf/bookset/internal/style"
)

func TestNormalizeTimelineStructures(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Chapter\n\n**Then:** Earlier.\n\n**Now:** Today.\n\n## Timeline\n\n- **1840:** First.\n- **1850:** Second.\n\n## Section\n\nText.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	cfg, ok := style.Preset("timeline-trade", "en")
	if !ok {
		t.Fatal("preset unavailable")
	}
	cfg.ChapterLabel = "CHAPTER"
	cfg.SectionNumbering = true
	normalized := Normalize(doc, cfg)
	want := []Kind{ChapterOpener, ThenNow, ThenNow, Timeline, Section, Paragraph}
	if len(normalized.Blocks) != len(want) {
		t.Fatalf("blocks=%d: %#v", len(normalized.Blocks), normalized.Blocks)
	}
	for i, kind := range want {
		if normalized.Blocks[i].Kind != kind {
			t.Errorf("block %d = %q, want %q", i, normalized.Blocks[i].Kind, kind)
		}
	}
	if normalized.Blocks[3].Children[0].Date != "1840" {
		t.Errorf("timeline date = %q", normalized.Blocks[3].Children[0].Date)
	}
}

func TestNormalizeGenericStyleKeepsOrdinaryHeadings(t *testing.T) {
	doc, parseIssues := markdown.Parse([]byte("# Chapter\n\n## Section\n\n**Then:** ordinary text.\n"))
	if issues := markdown.Validate(doc, parseIssues); len(issues) != 0 {
		t.Fatal(markdown.FormatIssues(issues))
	}
	normalized := Normalize(doc, style.Trade("en"))
	if normalized.Blocks[0].Kind != Heading || normalized.Blocks[1].Kind != Heading || normalized.Blocks[2].Kind != Paragraph {
		t.Fatalf("unexpected generic normalization: %#v", normalized.Blocks)
	}
}
