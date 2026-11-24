package screen

import (
	"strings"
	"testing"
)

func line(row, col int, text string) []any {
	var cells []any
	for _, r := range text {
		cells = append(cells, []any{string(r), int64(0)})
	}
	return []any{"grid_line", []any{int64(1), int64(row), int64(col), cells, false}}
}

func TestScrollHooksKeepPixelsAndCellsInStep(t *testing.T) {
	s := New(4, 4)
	s.HandleRedraw([]any{
		line(0, 0, "aaaa"), line(1, 0, "bbbb"), line(2, 0, "cccc"), line(3, 0, "dddd"),
		[]any{"flush", []any{}},
	})
	s.TakeDirty()
	var before int
	var got []int
	s.BeforeScroll = func() {
		before++
		// The renderer draws the dirty rows here: row 3 was changed
		// before the scroll and must be drawn before pixels move.
		if d := s.TakeDirty(); len(d) != 1 || d[0] != 3 {
			t.Errorf("dirty before scroll = %v, want [3]", d)
		}
	}
	s.AfterScroll = func(top, bot, left, right, rows int) { got = []int{top, bot, left, right, rows} }
	s.HandleRedraw([]any{
		line(3, 0, "DDDD"),
		[]any{"grid_scroll", []any{int64(1), int64(0), int64(4), int64(0), int64(4), int64(1)}},
		[]any{"flush", []any{}},
	})
	if before != 1 || len(got) != 5 || got[4] != 1 {
		t.Fatalf("hooks: before=%d after=%v", before, got)
	}
	// Cells moved like pixels; nothing is left dirty by the scroll itself.
	if d := s.TakeDirty(); len(d) != 0 {
		t.Errorf("dirty after scroll = %v, want none", d)
	}
	want := "bbbb\ncccc\nDDDD\nDDDD\n"
	if s.Dump() != want {
		t.Errorf("dump = %q, want %q", s.Dump(), want)
	}
}

func TestScrollWithoutHooksMarksRows(t *testing.T) {
	s := New(3, 3)
	s.TakeDirty()
	s.HandleRedraw([]any{[]any{"grid_scroll", []any{int64(1), int64(0), int64(3), int64(0), int64(3), int64(-1)}}})
	if d := s.TakeDirty(); len(d) != 3 {
		t.Errorf("dirty = %v", d)
	}
}

func TestWideCell(t *testing.T) {
	s := New(6, 1)
	s.HandleRedraw([]any{[]any{"grid_line", []any{int64(1), int64(0), int64(0),
		[]any{[]any{"日", int64(0)}, []any{""}, []any{"a"}}, false}}})
	if s.CellWidth(0, 0) != 2 || s.CellWidth(0, 1) != 0 || s.CellWidth(0, 2) != 1 {
		t.Fatal("widths")
	}
	if !strings.HasPrefix(s.Dump(), "日a") {
		t.Fatalf("dump %q", s.Dump())
	}
}
