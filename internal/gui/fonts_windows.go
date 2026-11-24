//go:build windows

package gui

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"ngw/internal/win32"
)

// defaultFaces are tried, in order, when 'guifont' is empty. BIZ UDGothic
// ships with Windows 10 1809 and later and has half-width Latin and
// full-width kana and kanji at exactly 1:2, which is what a cell grid
// needs for Japanese.
var defaultFaces = []string{"BIZ UDGothic", "MS Gothic", "Cascadia Mono", "Consolas"}

// fallbackFaces are searched for characters the chosen fonts lack.
var fallbackFaces = []string{
	"BIZ UDGothic", "MS Gothic", "Yu Gothic", "Meiryo",
	"Symbols Nerd Font Mono", "Segoe UI Symbol", "Cambria Math",
	"Segoe UI Emoji", "Segoe UI", "SimSun-ExtB", "Malgun Gothic",
	"Microsoft YaHei", "Nirmala UI", "Ebrima", "Leelawadee UI",
}

const defaultPoints = 12

// Variants of a face.
const (
	vRegular = iota
	vBold
	vItalic
	vBoldItalic
)

func variantOf(bold, italic bool) int {
	v := vRegular
	if bold {
		v |= vBold
	}
	if italic {
		v |= vItalic
	}
	return v
}

type face struct {
	name   string
	fonts  [4]uintptr // created on first use
	ascent int32
	adv    map[string]int32 // advance width of a cell's text (regular)
	avgW   int32
	scaled map[int32]uintptr // horizontally condensed copies, by lfWidth|variant<<24
}

// Fonts holds the GDI fonts and the cell metrics derived from the primary
// font. UI thread only.
type Fonts struct {
	dc      uintptr // measuring DC
	dcFont  uintptr
	dpi     uint32
	points  float64
	quality byte
	bold    bool // guifont ":b": everything bold
	italic  bool

	faces    []*face // primary first, then guifontwide, then fallbacks
	nPrimary int
	wide     []int // indexes tried first for double-width cells
	pick     map[pickKey]int

	CellW, CellH int32
	Baseline     int32 // from the top of a cell
	Linespace    int32
	Ascent       int32
	Descent      int32
	LogFont      win32.LOGFONTW // the primary font, for the IME
}

type pickKey struct {
	text string
	wide bool
}

// NewFonts creates fonts from 'guifont' and 'guifontwide' values.
func NewFonts(guifont, guifontwide string, linespace int, dpi uint32) *Fonts {
	f := &Fonts{dc: win32.CreateCompatibleDC(0)}
	f.Set(guifont, guifontwide, linespace, dpi)
	return f
}

func (f *Fonts) release() {
	win32.SelectObject(f.dc, win32.GetStockObject(17)) // DEFAULT_GUI_FONT
	f.dcFont = 0
	for _, fc := range f.faces {
		for i, h := range fc.fonts {
			if h != 0 {
				win32.DeleteObject(h)
				fc.fonts[i] = 0
			}
		}
		for _, h := range fc.scaled {
			win32.DeleteObject(h)
		}
	}
	f.faces = nil
}

// Set (re)creates the fonts. It reports whether the cell size changed.
func (f *Fonts) Set(guifont, guifontwide string, linespace int, dpi uint32) bool {
	oldW, oldH := f.CellW, f.CellH
	f.release()
	spec := ParseGuifont(guifont)
	wspec := ParseGuifont(guifontwide)
	f.dpi = dpi
	f.points = spec.Size
	if f.points == 0 {
		f.points = defaultPoints
	}
	f.bold, f.italic = spec.Bold, spec.Italic
	switch spec.Quality {
	case "antialiased":
		f.quality = win32.ANTIALIASED_QUALITY
	case "none":
		f.quality = win32.NONANTIALIASED_QUALITY
	default:
		f.quality = win32.CLEARTYPE_QUALITY
	}
	f.Linespace = int32(linespace)
	f.pick = map[pickKey]int{}

	seen := map[string]bool{}
	add := func(name string) int {
		key := strings.ToLower(name)
		for i, fc := range f.faces {
			if strings.ToLower(fc.name) == key {
				return i
			}
		}
		if seen[key] || !win32.FontExists(f.dc, name) {
			seen[key] = true
			return -1
		}
		seen[key] = true
		f.faces = append(f.faces, &face{name: name, adv: map[string]int32{}, scaled: map[int32]uintptr{}})
		return len(f.faces) - 1
	}
	for _, n := range spec.Faces {
		add(n)
	}
	if len(f.faces) == 0 {
		for _, n := range defaultFaces {
			if add(n) >= 0 {
				break
			}
		}
	}
	if len(f.faces) == 0 {
		// Nothing we know is installed: let GDI pick a monospace font.
		f.faces = append(f.faces, &face{name: "", adv: map[string]int32{}, scaled: map[int32]uintptr{}})
	}
	f.nPrimary = len(f.faces)
	f.wide = nil
	for _, n := range wspec.Faces {
		if i := add(n); i >= 0 {
			f.wide = append(f.wide, i)
		}
	}
	for _, n := range fallbackFaces {
		add(n)
	}

	// Cell metrics come from the primary font.
	p := f.faces[0]
	f.selectFace(p, vRegular)
	var tm win32.TEXTMETRICW
	win32.GetTextMetrics(f.dc, &tm)
	m := win32.GetTextExtent(f.dc, utf16.Encode([]rune("M")))
	f.CellW = max(1, m.CX)
	f.Ascent, f.Descent = tm.Ascent, tm.Descent
	f.CellH = max(1, tm.Height+f.Linespace)
	f.Baseline = tm.Ascent + f.Linespace/2
	f.LogFont = f.logFont(p.name, vRegular, 0)
	for _, fc := range f.faces {
		f.selectFace(fc, vRegular)
		var t win32.TEXTMETRICW
		win32.GetTextMetrics(f.dc, &t)
		fc.ascent, fc.avgW = t.Ascent, t.AveCharWidth
	}
	return f.CellW != oldW || f.CellH != oldH
}

func (f *Fonts) logFont(name string, v int, width int32) win32.LOGFONTW {
	var lf win32.LOGFONTW
	lf.Height = -pixels(f.points, f.dpi)
	lf.Width = width
	lf.Weight = win32.FW_NORMAL
	if v&vBold != 0 || f.bold {
		lf.Weight = win32.FW_BOLD
	}
	if v&vItalic != 0 || f.italic {
		lf.Italic = 1
	}
	lf.CharSet = win32.DEFAULT_CHARSET
	lf.OutPrecision = win32.OUT_TT_PRECIS
	lf.ClipPrecision = win32.CLIP_DEFAULT_PRECIS
	lf.Quality = f.quality
	lf.PitchAndFamily = win32.FIXED_PITCH | win32.FF_MODERN
	u := utf16.Encode([]rune(name))
	if len(u) > 31 {
		u = u[:31]
	}
	copy(lf.FaceName[:], u)
	return lf
}

func (f *Fonts) font(fc *face, v int) uintptr {
	if fc.fonts[v] == 0 {
		lf := f.logFont(fc.name, v, 0)
		fc.fonts[v] = win32.CreateFontIndirect(&lf)
	}
	return fc.fonts[v]
}

// condensed returns fc squeezed horizontally so that text whose advance is
// adv fits in slot pixels.
func (f *Fonts) condensed(fc *face, v int, adv, slot int32) uintptr {
	w := max(1, fc.avgW*slot/adv)
	key := w | int32(v)<<24
	if h, ok := fc.scaled[key]; ok {
		return h
	}
	lf := f.logFont(fc.name, v, w)
	h := win32.CreateFontIndirect(&lf)
	fc.scaled[key] = h
	return h
}

func (f *Fonts) selectFace(fc *face, v int) {
	h := f.font(fc, v)
	if f.dcFont != h {
		win32.SelectObject(f.dc, h)
		f.dcFont = h
	}
}

// Pick returns the face to draw a cell's text with. wide is true for
// double-width cells: 'guifontwide' is tried first for them.
func (f *Fonts) Pick(text string, wide bool) int {
	if len(text) == 1 && text[0] < 0x80 {
		return 0 // ASCII: the primary font, always
	}
	k := pickKey{text, wide && len(f.wide) > 0}
	if i, ok := f.pick[k]; ok {
		return i
	}
	i := f.search(text, k.wide)
	f.pick[k] = i
	return i
}

func (f *Fonts) search(text string, wide bool) int {
	r, _ := utf8.DecodeRuneInString(text)
	u := utf16.Encode([]rune(text))
	if r > 0xFFFF {
		// GetGlyphIndices cannot check characters outside the BMP.
		return f.byName(astralFace(r))
	}
	try := func(i int) bool {
		f.selectFace(f.faces[i], vRegular)
		return win32.HasGlyphs(f.dc, u[:1])
	}
	if wide {
		for _, i := range f.wide {
			if try(i) {
				return i
			}
		}
	}
	for i := range f.faces {
		if try(i) {
			return i
		}
	}
	return 0 // GDI's own font linking may still find a glyph
}

func astralFace(r rune) string {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF:
		return "Segoe UI Emoji"
	case r >= 0xF0000:
		return "Symbols Nerd Font Mono"
	case r >= 0x20000 && r <= 0x3FFFF:
		return "SimSun-ExtB"
	}
	return "Segoe UI Symbol"
}

func (f *Fonts) byName(name string) int {
	for i, fc := range f.faces {
		if strings.EqualFold(fc.name, name) {
			return i
		}
	}
	return 0
}

// Advance returns the width GDI gives text in face i (regular).
func (f *Fonts) Advance(i int, text string, u []uint16) int32 {
	if i == 0 && len(text) == 1 && text[0] < 0x80 {
		return f.CellW
	}
	fc := f.faces[i]
	if a, ok := fc.adv[text]; ok {
		return a
	}
	f.selectFace(fc, vRegular)
	a := win32.GetTextExtent(f.dc, u).CX
	fc.adv[text] = a
	return a
}

// Font returns the HFONT for face i and variant v.
func (f *Fonts) Font(i, v int) uintptr { return f.font(f.faces[i], v) }

// Condensed returns face i, variant v squeezed so adv fits in slot.
func (f *Fonts) Condensed(i, v int, adv, slot int32) uintptr {
	return f.condensed(f.faces[i], v, adv, slot)
}

// PrimaryName returns the face actually used for the grid.
func (f *Fonts) PrimaryName() string { return f.faces[0].name }

// Points returns the font size in points.
func (f *Fonts) Points() float64 { return f.points }
