//go:build windows

package gui

import (
	"unicode/utf16"
	"unicode/utf8"

	"ngw/internal/screen"
	"ngw/internal/width"
	"ngw/internal/win32"
)

// renderer keeps Neovim's grid as pixels in an off-screen bitmap. Only
// rows Neovim changed are drawn again, and grid_scroll moves pixels with
// one BitBlt instead of drawing the scrolled rows. The window is updated
// from the bitmap; the cursor and the input method's preedit are drawn on
// top of it while painting, so they never get into the bitmap.
type renderer struct {
	hwnd  uintptr
	dc    uintptr // memory DC holding bmp
	bmp   uintptr
	oldBm uintptr
	bw    int32
	bh    int32
	font  uintptr // font selected in the DC being drawn on

	scr   *screen.Screen
	fonts *Fonts

	u16 []uint16
	dx  []int32
	pts []win32.POINT

	pens map[uint32]uintptr

	// rows invalidated in the window since the last present
	invTop, invBot int
}

func newRenderer(hwnd uintptr, scr *screen.Screen, fonts *Fonts) *renderer {
	r := &renderer{hwnd: hwnd, scr: scr, fonts: fonts, pens: map[uint32]uintptr{}, invTop: -1}
	wdc := win32.GetDC(hwnd)
	r.dc = win32.CreateCompatibleDC(wdc)
	win32.ReleaseDC(hwnd, wdc)
	win32.SetBkMode(r.dc, win32.TRANSPARENT)
	win32.SetTextAlign(r.dc, win32.TA_BASELINE|win32.TA_LEFT)
	scr.BeforeScroll = func() { r.drawDirty() }
	scr.AfterScroll = r.scrolled
	return r
}

func colorref(c screen.Color) uint32 {
	rr, g, b := c.RGB()
	return uint32(rr) | uint32(g)<<8 | uint32(b)<<16
}

// ensure makes the bitmap at least w×h and at least as large as the grid.
// A new bitmap means every row has to be drawn again.
func (r *renderer) ensure(w, h int32) {
	w = max(w, int32(r.scr.Width)*r.fonts.CellW, 1)
	h = max(h, int32(r.scr.Height)*r.fonts.CellH, 1)
	if w <= r.bw && h <= r.bh && r.bmp != 0 {
		return
	}
	// Grow with slack so dragging the window edge does not reallocate on
	// every step.
	if r.bmp != 0 {
		w, h = max(w, r.bw+r.bw/4), max(h, r.bh+r.bh/4)
	}
	wdc := win32.GetDC(r.hwnd)
	bmp := win32.CreateCompatibleBitmap(wdc, w, h)
	win32.ReleaseDC(r.hwnd, wdc)
	old := win32.SelectObject(r.dc, bmp)
	if r.bmp != 0 {
		win32.DeleteObject(r.bmp)
	} else {
		r.oldBm = old
	}
	r.bmp, r.bw, r.bh = bmp, w, h
	r.font = 0
	r.scr.MarkAllDirty()
}

func (r *renderer) invalidateRows(top, bot int) {
	if r.invTop < 0 {
		r.invTop, r.invBot = top, bot
		return
	}
	r.invTop, r.invBot = min(r.invTop, top), max(r.invBot, bot)
}

// drawDirty draws the rows that changed into the bitmap.
func (r *renderer) drawDirty() {
	r.ensure(0, 0)
	rows := r.scr.TakeDirty()
	if len(rows) == 0 {
		return
	}
	if len(rows) == r.scr.Height {
		// Everything: also the margin right of and below the grid.
		rc := win32.RECT{Right: r.bw, Bottom: r.bh}
		win32.FillRect(r.dc, &rc, colorref(r.scr.DefaultBG))
	}
	for _, row := range rows {
		r.drawRow(r.dc, row)
	}
	st.RowsDrawn += len(rows)
	r.invalidateRows(rows[0], rows[len(rows)-1]+1)
}

func (r *renderer) scrolled(top, bot, left, right, rows int) {
	st.ScrollBlits++
	cw, ch := r.fonts.CellW, r.fonts.CellH
	x, w := int32(left)*cw, int32(right-left)*cw
	if rows > 0 {
		win32.BitBlt(r.dc, x, int32(top)*ch, w, int32(bot-top-rows)*ch, r.dc, x, int32(top+rows)*ch)
	} else {
		win32.BitBlt(r.dc, x, int32(top-rows)*ch, w, int32(bot-top+rows)*ch, r.dc, x, int32(top)*ch)
	}
	r.invalidateRows(top, bot)
}

// flushInvalid sends the rows drawn since the last call to the window.
func (r *renderer) flushInvalid() {
	if r.invTop < 0 {
		return
	}
	ch := r.fonts.CellH
	rc := win32.RECT{Top: int32(r.invTop) * ch, Right: 1 << 30, Bottom: int32(r.invBot) * ch}
	if r.invBot >= r.scr.Height {
		rc.Bottom = 1 << 30
	}
	win32.InvalidateRect(r.hwnd, &rc)
	r.invTop = -1
}

func (r *renderer) selectFont(dc, h uintptr) {
	if r.font != h {
		win32.SelectObject(dc, h)
		r.font = h
	}
}

func (r *renderer) pen(c uint32) uintptr {
	p, ok := r.pens[c]
	if !ok {
		if len(r.pens) > 64 {
			for k, v := range r.pens {
				win32.DeleteObject(v)
				delete(r.pens, k)
			}
		}
		p = win32.CreatePen(win32.PS_SOLID, 1, c)
		r.pens[c] = p
	}
	return p
}

// drawRow draws one grid row: backgrounds, then text, then lines. A whole
// row is always drawn so that wide or overhanging glyphs leave nothing
// behind.
func (r *renderer) drawRow(dc uintptr, row int) {
	cells := r.scr.Row(row)
	cw, ch := r.fonts.CellW, r.fonts.CellH
	y := int32(row) * ch
	// Backgrounds, one fill per run of the same color.
	for c := 0; c < len(cells); {
		_, bg, _ := r.scr.Colors(cells[c].HL)
		e := c + 1
		for e < len(cells) {
			_, b2, _ := r.scr.Colors(cells[e].HL)
			if b2 != bg {
				break
			}
			e++
		}
		rc := win32.RECT{Left: int32(c) * cw, Top: y, Right: int32(e) * cw, Bottom: y + ch}
		if e == len(cells) && dc == r.dc {
			rc.Right = max(rc.Right, r.bw)
		}
		win32.FillRect(dc, &rc, colorref(bg))
		c = e
	}
	clip := win32.RECT{Left: 0, Top: y, Right: max(r.bw, int32(len(cells))*cw), Bottom: y + ch}
	r.drawText(dc, cells, row, 0, len(cells), clip, -1)
	r.drawLines(dc, cells, row)
}

// textStyle is what a run of text has in common: color and variant.
// Highlights that differ only in background or lines draw text the same
// way, so they share runs.
type textStyle struct {
	fg screen.Color
	v  int
}

func (r *renderer) style(hl int, fgOverride screen.Color) textStyle {
	a := r.scr.Attr(hl)
	fg, _, _ := r.scr.Colors(hl)
	if fgOverride >= 0 {
		fg = fgOverride
	}
	return textStyle{fg, variantOf(a.Bold, a.Italic)}
}

// drawText draws the text of cells [from, to) of a row, with as few GDI
// calls as possible: one ExtTextOutW per run of cells with the same text
// style and font. Blank cells do not break a run (the per-character
// advances skip over them), since backgrounds are drawn separately.
// fgOverride, if not negative, replaces every foreground color (the
// block cursor).
func (r *renderer) drawText(dc uintptr, cells []screen.Cell, row, from, to int, clip win32.RECT, fgOverride screen.Color) {
	cw := r.fonts.CellW
	baseline := int32(row)*r.fonts.CellH + r.fonts.Baseline
	lastColor := screen.Color(-2)
	setColor := func(fg screen.Color) {
		if fg != lastColor {
			win32.SetTextColor(dc, colorref(fg))
			lastColor = fg
		}
	}
	for c := from; c < to; {
		cell := cells[c]
		if cell.Text == "" || cell.Text == " " {
			c++
			continue
		}
		wide := c+1 < len(cells) && cells[c+1].Text == ""
		text := glyphText(cell.Text)
		fi := r.fonts.Pick(text, wide)
		st0 := r.style(cell.HL, fgOverride)
		if solo(text) {
			// Characters outside the BMP and clusters of several code
			// points (combining marks) are drawn on their own, at their
			// cell: how ExtTextOutW spreads lpDx over surrogate pairs and
			// combining marks is not something to rely on.
			w := 1
			if wide {
				w = 2
			}
			setColor(st0.fg)
			r.drawSolo(dc, fi, st0.v, text, int32(c)*cw, int32(w)*cw, baseline, clip)
			c += w
			continue
		}
		lastHL := cell.HL
		r.u16, r.dx = r.u16[:0], r.dx[:0]
		x0 := int32(-1)
		var prev int32
		e := c
		for e < to {
			cc := cells[e]
			if cc.Text == "" || cc.Text == " " {
				e++
				continue
			}
			if cc.HL != lastHL {
				if r.style(cc.HL, fgOverride) != st0 {
					break
				}
				lastHL = cc.HL
			}
			w := 1
			if e+1 < len(cells) && cells[e+1].Text == "" {
				w = 2
			}
			t := glyphText(cc.Text)
			if solo(t) || r.fonts.Pick(t, w == 2) != fi {
				break
			}
			slot := int32(w) * cw
			start := len(r.u16)
			r.u16 = appendUTF16(r.u16, t)
			adv := r.fonts.Advance(fi, t, r.u16[start:])
			if adv > slot+1 {
				// Too wide for its cells (an ambiguous-width character
				// in a Japanese font with 'ambiwidth' single, a Nerd Font
				// icon, ...): drawn alone, squeezed into its cells.
				r.u16 = r.u16[:start]
				if x0 < 0 {
					setColor(st0.fg)
					r.drawSolo(dc, fi, st0.v, t, int32(e)*cw, slot, baseline, clip)
					e += w
				}
				break
			}
			off := int32(0)
			if fi != 0 && adv < slot {
				off = (slot - adv) / 2 // center fallback glyphs
			}
			origin := int32(e)*cw + off
			if x0 < 0 {
				x0 = origin
			} else {
				r.dx[len(r.dx)-1] += origin - prev
			}
			r.dx = append(r.dx, 0)
			prev = origin
			e += w
		}
		if len(r.u16) > 0 {
			setColor(st0.fg)
			r.selectFont(dc, r.fonts.Font(fi, st0.v))
			win32.ExtTextOut(dc, x0, baseline, win32.ETO_CLIPPED, &clip, r.u16, r.dx)
			st.TextCalls++
		}
		c = max(e, c+1)
	}
}

// drawSolo draws one cell's text by itself in the slot [x, x+slot):
// centered if it is narrower (fallback fonts), squeezed if it is wider.
func (r *renderer) drawSolo(dc uintptr, fi, v int, text string, x, slot, baseline int32, clip win32.RECT) {
	r.u16 = appendUTF16(r.u16[:0], text)
	adv := r.fonts.Advance(fi, text, r.u16)
	h := r.fonts.Font(fi, v)
	ox := x
	switch {
	case adv > slot+1:
		h = r.fonts.Condensed(fi, v, adv, slot)
	case adv < slot && (fi != 0 || slot > r.fonts.CellW):
		ox += (slot - adv) / 2
	}
	r.selectFont(dc, h)
	rc := clip
	rc.Left, rc.Right = x, x+slot
	win32.ExtTextOut(dc, ox, baseline, win32.ETO_CLIPPED, &rc, r.u16, nil)
	st.TextCalls++
}

func appendUTF16(b []uint16, s string) []uint16 {
	for len(s) > 0 {
		r, n := utf8.DecodeRuneInString(s)
		s = s[n:]
		if r >= 0x10000 {
			r1, r2 := utf16.EncodeRune(r)
			b = append(b, uint16(r1), uint16(r2))
		} else {
			b = append(b, uint16(r))
		}
	}
	return b
}

// drawLines draws underlines, undercurls and strikethrough.
func (r *renderer) drawLines(dc uintptr, cells []screen.Cell, row int) {
	cw, ch := r.fonts.CellW, r.fonts.CellH
	top := int32(row) * ch
	base := top + r.fonts.Baseline
	th := max(1, ch/16)
	uy := min(base+max(1, r.fonts.Descent/2), top+ch-th)
	for c := 0; c < len(cells); {
		hl := cells[c].HL
		e := c + 1
		for e < len(cells) && cells[e].HL == hl {
			e++
		}
		a := r.scr.Attr(hl)
		if a.Underline || a.Undercurl || a.Underdouble || a.Underdotted || a.Underdashed || a.Strikethrough {
			fg, _, sp := r.scr.Colors(hl)
			x0, x1 := int32(c)*cw, int32(e)*cw
			lc := sp
			if a.SP == screen.Default && !a.Undercurl {
				lc = fg
			}
			col := colorref(lc)
			switch {
			case a.Undercurl:
				r.curl(dc, x0, x1, top+ch-1, col)
			case a.Underdouble:
				rc := win32.RECT{Left: x0, Top: uy - th - 1, Right: x1, Bottom: uy - 1}
				win32.FillRect(dc, &rc, col)
				rc = win32.RECT{Left: x0, Top: uy + 1, Right: x1, Bottom: uy + 1 + th}
				win32.FillRect(dc, &rc, col)
			case a.Underdotted, a.Underdashed:
				step, on := int32(2), int32(1)
				if a.Underdashed {
					step, on = max(4, cw/2+2), max(2, cw/2-1)
				}
				for x := x0; x < x1; x += step {
					rc := win32.RECT{Left: x, Top: uy, Right: min(x+on, x1), Bottom: uy + th}
					win32.FillRect(dc, &rc, col)
				}
			case a.Underline:
				rc := win32.RECT{Left: x0, Top: uy, Right: x1, Bottom: uy + th}
				win32.FillRect(dc, &rc, col)
			}
			if a.Strikethrough {
				sy := base - r.fonts.Ascent/3
				rc := win32.RECT{Left: x0, Top: sy, Right: x1, Bottom: sy + th}
				win32.FillRect(dc, &rc, colorref(fg))
			}
		}
		c = e
	}
}

func (r *renderer) curl(dc uintptr, x0, x1, bottom int32, col uint32) {
	r.pts = r.pts[:0]
	for x := x0; x <= x1; x += 2 {
		y := bottom
		if (x/2)%2 == 0 {
			y = bottom - 2
		}
		r.pts = append(r.pts, win32.POINT{X: x, Y: y})
	}
	old := win32.SelectObject(dc, r.pen(col))
	win32.Polyline(dc, r.pts)
	win32.SelectObject(dc, old)
}

// cursorRect returns the cursor's rectangle in window pixels.
func (r *renderer) cursorRect() win32.RECT {
	s := r.scr
	cw, ch := r.fonts.CellW, r.fonts.CellH
	row, col := s.CursorRow, s.CursorCol
	w := int32(1)
	if s.CellWidth(row, col) == 2 {
		w = 2
	}
	return win32.RECT{Left: int32(col) * cw, Top: int32(row) * ch, Right: int32(col)*cw + w*cw, Bottom: int32(row+1) * ch}
}

// drawCursor draws the cursor on dc (the window), over the bitmap.
func (r *renderer) drawCursor(dc uintptr, focused bool) {
	s := r.scr
	if s.Busy || s.CursorRow >= s.Height || s.CursorCol >= s.Width {
		return
	}
	row, col := s.CursorRow, s.CursorCol
	cells := s.Row(row)
	if cells[col].Text == "" && col > 0 {
		col-- // right half of a wide character
	}
	fg, bg := s.CursorColors(cells[col].HL)
	rc := r.cursorRect()
	m := s.CurrentMode()
	if !focused {
		for _, e := range []win32.RECT{
			{Left: rc.Left, Top: rc.Top, Right: rc.Right, Bottom: rc.Top + 1},
			{Left: rc.Left, Top: rc.Bottom - 1, Right: rc.Right, Bottom: rc.Bottom},
			{Left: rc.Left, Top: rc.Top, Right: rc.Left + 1, Bottom: rc.Bottom},
			{Left: rc.Right - 1, Top: rc.Top, Right: rc.Right, Bottom: rc.Bottom},
		} {
			win32.FillRect(dc, &e, colorref(bg))
		}
		return
	}
	switch m.Shape {
	case screen.CursorVertical:
		w := max(1, r.fonts.CellW*int32(m.CellPercentage)/100)
		b := win32.RECT{Left: rc.Left, Top: rc.Top, Right: rc.Left + w, Bottom: rc.Bottom}
		win32.FillRect(dc, &b, colorref(bg))
	case screen.CursorHorizontal:
		h := max(1, r.fonts.CellH*int32(m.CellPercentage)/100)
		b := win32.RECT{Left: rc.Left, Top: rc.Bottom - h, Right: rc.Right, Bottom: rc.Bottom}
		win32.FillRect(dc, &b, colorref(bg))
	default:
		win32.FillRect(dc, &rc, colorref(bg))
		end := col + 1
		if end < len(cells) && cells[end].Text == "" {
			end++
		}
		r.drawText(dc, cells, row, col, end, rc, fg)
	}
}

// preedit is the text an input method is composing.
type preedit struct {
	text   string
	attrs  []byte // one per rune: ATTR_INPUT, ATTR_TARGET_CONVERTED, ...
	cursor int    // in runes
}

// preeditLayout places preedit cells from the cursor on, wrapping at the
// right edge. It returns each rune's (row, col) and width.
type pcell struct {
	row, col, w int
	text        string
	attr        byte
}

func (r *renderer) layoutPreedit(p *preedit, ambiDouble bool) []pcell {
	s := r.scr
	row, col := s.CursorRow, s.CursorCol
	var out []pcell
	i := 0
	for _, c := range width.Layout(p.text, ambiDouble) {
		if col+c.Width > s.Width {
			row, col = row+1, 0
		}
		var a byte
		if i < len(p.attrs) {
			a = p.attrs[i]
		}
		out = append(out, pcell{row: row, col: col, w: c.Width, text: c.Text, attr: a})
		col += c.Width
		i += utf8.RuneCountInString(c.Text)
	}
	return out
}

// preeditRect returns the rectangle the preedit covers.
func (r *renderer) preeditRect(cells []pcell) win32.RECT {
	if len(cells) == 0 {
		return win32.RECT{}
	}
	ch := r.fonts.CellH
	first, last := cells[0], cells[len(cells)-1]
	rc := win32.RECT{Top: int32(first.row) * ch, Bottom: int32(last.row+1) * ch, Left: 0, Right: 1 << 30}
	if first.row == last.row {
		rc.Left = int32(first.col) * r.fonts.CellW
		rc.Right = int32(last.col+last.w)*r.fonts.CellW + 2
	}
	return rc
}

// drawPreedit draws the composition at the cursor, like text being typed:
// default colors, underlined, the clause being converted reversed, and a
// bar at the input method's caret.
func (r *renderer) drawPreedit(dc uintptr, cells []pcell, caret int) {
	s := r.scr
	cw, ch := r.fonts.CellW, r.fonts.CellH
	fg, bg := s.DefaultFG, s.DefaultBG
	var buf [2]screen.Cell
	for _, c := range cells {
		if c.row >= s.Height {
			break
		}
		f, b := fg, bg
		target := c.attr == win32.ATTR_TARGET_CONVERTED || c.attr == win32.ATTR_TARGET_NOTCONVERTED
		if target {
			f, b = bg, fg
		}
		rc := win32.RECT{Left: int32(c.col) * cw, Top: int32(c.row) * ch, Right: int32(c.col+c.w) * cw, Bottom: int32(c.row+1) * ch}
		win32.FillRect(dc, &rc, colorref(b))
		buf[0] = screen.Cell{Text: c.text}
		cellsRow := buf[:1]
		if c.w == 2 {
			buf[1] = screen.Cell{}
			cellsRow = buf[:2]
		}
		// Draw as a fake two-cell row at the right place: shift x by
		// drawing into a row of its own with an origin offset.
		r.drawPreeditCell(dc, cellsRow, c, rc, f)
		th := int32(1)
		if c.attr == win32.ATTR_CONVERTED || target {
			th = max(2, ch/12)
		}
		u := win32.RECT{Left: rc.Left + 1, Top: rc.Bottom - th - 1, Right: rc.Right - 1, Bottom: rc.Bottom - 1}
		win32.FillRect(dc, &u, colorref(f))
	}
	// The caret.
	if caret >= 0 {
		var x, y int32
		if caret < len(cells) {
			x, y = int32(cells[caret].col)*cw, int32(cells[caret].row)*ch
		} else if len(cells) > 0 {
			l := cells[len(cells)-1]
			x, y = int32(l.col+l.w)*cw, int32(l.row)*ch
		}
		b := win32.RECT{Left: x, Top: y, Right: x + max(1, cw/8), Bottom: y + ch}
		win32.FillRect(dc, &b, colorref(fg))
	}
}

func (r *renderer) drawPreeditCell(dc uintptr, cells []screen.Cell, c pcell, rc win32.RECT, fg screen.Color) {
	cw := r.fonts.CellW
	fi := r.fonts.Pick(c.text, c.w == 2)
	r.u16 = appendUTF16(r.u16[:0], c.text)
	slot := int32(c.w) * cw
	adv := r.fonts.Advance(fi, c.text, r.u16)
	h := r.fonts.Font(fi, vRegular)
	off := int32(0)
	if adv > slot+1 {
		h = r.fonts.Condensed(fi, vRegular, adv, slot)
	} else if fi != 0 && adv < slot {
		off = (slot - adv) / 2
	}
	r.selectFont(dc, h)
	win32.SetTextColor(dc, colorref(fg))
	r.dx = r.dx[:0]
	for range r.u16 {
		r.dx = append(r.dx, 0)
	}
	win32.ExtTextOut(dc, rc.Left+off, rc.Top+r.fonts.Baseline, win32.ETO_CLIPPED, &rc, r.u16, r.dx)
	_ = cells
}
