package widgets

import (
	"testing"

	"github.com/ryoku-dev/ryoku/docs/lithic/library/go/kit"
)

func TestHeroMetaCollapsesSpanBoundaryWhitespace(t *testing.T) {
	t.Parallel()
	spans := heroMeta([]kit.Span{
		{Text: "Artist"},
		{Text: " · "},
		{Text: "8 ", Mono: true},
		{Text: " songs"},
	})
	if len(spans) != 4 {
		t.Fatalf("heroMeta returned %d spans, want 4", len(spans))
	}
	got := ""
	for _, run := range spans {
		if run.gap {
			got += " "
		}
		got += run.span.Text
	}
	if got != "Artist · 8 songs" {
		t.Fatalf("joined metadata = %q, want %q", got, "Artist · 8 songs")
	}
	if !spans[3].gap || spans[2].span.Text != "8" || spans[3].span.Text != "songs" {
		t.Fatalf("mono-to-sans boundary was not converted to a fixed gap: %#v", spans)
	}
}
