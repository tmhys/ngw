package gui

import (
	"encoding/json"
	"os"
	"time"
)

// stats records where the UI thread spends its time, for benchmarks.
// It is enabled by setting NGW_STATS to a file path; the numbers are
// written there as JSON when ngw exits.
type stats struct {
	on                     bool
	Frames, Paints         int
	Apply, Draw, Paint     time.Duration // HandleRedraw, drawing rows, WM_PAINT
	ScrollBlits, RowsDrawn int
	Batches, Notifications int
}

var st = stats{on: os.Getenv("NGW_STATS") != ""}

func since(t time.Time) time.Duration { return time.Since(t) }

func writeStats() {
	path := os.Getenv("NGW_STATS")
	if path == "" {
		return
	}
	type out struct {
		Frames, Paints, ScrollBlits, RowsDrawn, Batches  int
		ApplyMs, DrawMs, PaintMs                         float64
		ApplyUsPerFrame, DrawUsPerFrame, PaintUsPerPaint float64
	}
	ms := func(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
	per := func(d time.Duration, n int) float64 {
		if n == 0 {
			return 0
		}
		return float64(d.Microseconds()) / float64(n)
	}
	b, _ := json.Marshal(out{
		Frames: st.Frames, Paints: st.Paints, ScrollBlits: st.ScrollBlits, RowsDrawn: st.RowsDrawn, Batches: st.Batches,
		ApplyMs: ms(st.Apply), DrawMs: ms(st.Draw), PaintMs: ms(st.Paint),
		ApplyUsPerFrame: per(st.Apply, st.Frames), DrawUsPerFrame: per(st.Draw, st.Frames), PaintUsPerPaint: per(st.Paint, st.Paints),
	})
	os.WriteFile(path, b, 0o644)
}
