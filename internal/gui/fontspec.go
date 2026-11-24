package gui

import (
	"strconv"
	"strings"
)

// FontSpec is a parsed 'guifont' (or 'guifontwide') value in GVim's
// Windows syntax: "Face:h12:b:i:qCLEARTYPE,Fallback Face". Underscores in
// a face name stand for spaces ("BIZ_UDGothic"). Options of the first
// entry that has them win.
type FontSpec struct {
	Faces   []string
	Size    float64 // points; 0 means not given
	Bold    bool
	Italic  bool
	Quality string // "cleartype", "antialiased", "none", "" (default)
}

// ParseGuifont parses a 'guifont' value. "\," and "\ " are literal.
func ParseGuifont(s string) FontSpec {
	var spec FontSpec
	for _, entry := range splitUnescaped(s, ',') {
		parts := strings.Split(entry, ":")
		name := strings.TrimSpace(strings.ReplaceAll(parts[0], "_", " "))
		if name != "" && name != "*" {
			spec.Faces = append(spec.Faces, name)
		}
		for _, p := range parts[1:] {
			if p == "" {
				continue
			}
			switch p[0] {
			case 'h':
				if v, err := strconv.ParseFloat(p[1:], 64); err == nil && v > 0 && spec.Size == 0 {
					spec.Size = v
				}
			case 'b':
				spec.Bold = true
			case 'i':
				spec.Italic = true
			case 'q':
				if spec.Quality == "" {
					switch strings.ToUpper(p[1:]) {
					case "CLEARTYPE", "CLEARTYPE_NATURAL":
						spec.Quality = "cleartype"
					case "ANTIALIASED", "PROOF", "DEFAULT":
						spec.Quality = "antialiased"
					case "NONANTIALIASED", "DRAFT":
						spec.Quality = "none"
					}
				}
			}
		}
	}
	return spec
}

func splitUnescaped(s string, sep byte) []string {
	var out []string
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && i+1 < len(s) {
			i++
			b.WriteByte(s[i])
			continue
		}
		if c == sep {
			out = append(out, b.String())
			b.Reset()
			continue
		}
		b.WriteByte(c)
	}
	if b.Len() > 0 || len(out) > 0 {
		out = append(out, b.String())
	}
	return out
}

// GuifontValue formats a face and size for ":set guifont=" with the
// spaces escaped, as the font dialog (":set guifont=*") sets it.
func GuifontValue(face string, size float64) string {
	v := strings.ReplaceAll(face, " ", "_") + ":h" + strconv.FormatFloat(size, 'f', -1, 64)
	return v
}

// pixels converts a size in points to a pixel em height at dpi.
func pixels(pt float64, dpi uint32) int32 {
	return int32(pt*float64(dpi)/72 + 0.5)
}
