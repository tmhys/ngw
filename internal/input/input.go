// Package input translates keyboard and mouse events into Neovim's key
// notation. It is derived from ng (https://github.com/Shougo/ng, MIT).
// It knows nothing about Win32 messages; the GUI maps virtual-key codes to
// Key first (FromVK), which keeps this logic testable on any platform.
package input

import "strings"

// Key identifies a non-text key.
type Key int

const (
	KeyNone Key = iota
	KeyEscape
	KeyEnter
	KeyTab
	KeyBackspace
	KeyDelete
	KeyInsert
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeySpace
	KeyF1 // KeyF1 + n-1 is Fn, up to F24
)

// Mod is a set of modifiers.
type Mod uint8

const (
	ModShift Mod = 1 << iota
	ModCtrl
	ModAlt
	ModSuper
)

var keyNames = map[Key]string{
	KeyEscape:    "Esc",
	KeyEnter:     "CR",
	KeyTab:       "Tab",
	KeyBackspace: "BS",
	KeyDelete:    "Del",
	KeyInsert:    "Insert",
	KeyHome:      "Home",
	KeyEnd:       "End",
	KeyPageUp:    "PageUp",
	KeyPageDown:  "PageDown",
	KeyUp:        "Up",
	KeyDown:      "Down",
	KeyLeft:      "Left",
	KeyRight:     "Right",
	KeySpace:     "Space",
}

func keyName(k Key) string {
	if k >= KeyF1 && k < KeyF1+24 {
		return "F" + itoa(int(k-KeyF1)+1)
	}
	return keyNames[k]
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func modPrefix(m Mod, withShift bool) string {
	var b strings.Builder
	if m&ModCtrl != 0 {
		b.WriteString("C-")
	}
	if withShift && m&ModShift != 0 {
		b.WriteString("S-")
	}
	if m&ModAlt != 0 {
		b.WriteString("M-")
	}
	if m&ModSuper != 0 {
		b.WriteString("D-")
	}
	return b.String()
}

// SpecialKey returns the notation for a non-text key, e.g. "<C-Up>". It
// returns "" for KeyNone. A plain space is sent as text instead.
func SpecialKey(k Key, m Mod) string {
	if k == KeyNone {
		return ""
	}
	if k == KeySpace && m&(ModCtrl|ModAlt|ModSuper|ModShift) == 0 {
		return "" // arrives as text input
	}
	name := keyName(k)
	if name == "" {
		return ""
	}
	return "<" + modPrefix(m, true) + name + ">"
}

// ChordKey returns the notation for a printable key pressed together with
// Ctrl, Alt or Super, e.g. "<C-a>" or "<M-x>". ch is the character the key
// produces without modifiers (the SDL keycode for printable keys). It
// returns "" when no such modifier is held, in which case the text input
// event carries the character.
func ChordKey(ch rune, m Mod) string {
	if m&(ModCtrl|ModAlt|ModSuper) == 0 || ch < 0x20 || ch == 0x7f {
		return ""
	}
	// Shift is folded into the character for letters: Ctrl-Shift-a is
	// "<C-S-a>" only when the user really wants the distinction; Vim
	// treats <C-A> and <C-a> the same, so keep the base letter.
	switch {
	case m&ModCtrl != 0 && m&(ModAlt|ModSuper) == 0:
		// Keys that terminals send as control characters.
		switch ch {
		case '6', '^':
			return "<C-^>"
		case '2', '@':
			return "<C-@>"
		case '_':
			return "<C-_>"
		}
	}
	name := string(ch)
	switch ch {
	case '<':
		name = "lt"
	case '\\':
		name = "Bslash"
	case '|':
		name = "Bar"
	case ' ':
		name = "Space"
	}
	return "<" + modPrefix(m, m&ModShift != 0 && !isLetter(ch)) + name + ">"
}

func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// Text escapes typed text for nvim_input: only "<" is special.
func Text(s string) string {
	return strings.ReplaceAll(s, "<", "<lt>")
}

// MouseModifier returns the modifier string for nvim_input_mouse.
func MouseModifier(m Mod) string {
	var b strings.Builder
	if m&ModCtrl != 0 {
		b.WriteString("C")
	}
	if m&ModShift != 0 {
		b.WriteString("S")
	}
	if m&ModAlt != 0 {
		b.WriteString("A")
	}
	return b.String()
}

// IsPaste reports whether the key is Ctrl+Shift+V, which pastes the
// clipboard as in terminal emulators. ch is the unshifted character.
func IsPaste(ch rune, m Mod) bool {
	return (ch == 'v' || ch == 'V') && m&(ModCtrl|ModShift|ModAlt|ModSuper) == ModCtrl|ModShift
}

// IMEWanted reports whether the input method should be active in a Neovim
// mode (a name from mode_info_set, as in mode_change). With insertOnly
// (the default, g:ng_ime_insert_only) it is active only where text is
// typed: Insert and Replace mode, the command line and Terminal mode. In
// Normal, Visual and Operator-pending mode keys then reach Neovim
// directly, even if the input method is switched on.
func IMEWanted(mode string, insertOnly bool) bool {
	if !insertOnly {
		return true
	}
	switch mode {
	case "insert", "replace", "showmatch", "terminal",
		"cmdline_normal", "cmdline_insert", "cmdline_replace":
		return true
	}
	return false
}

// DirectText returns the text a key types when the input method is off,
// or "" if the key types nothing by itself: ch is the character the key
// types with Shift, CapsLock and AltGr applied (SDL_GetKeyFromScancode;
// the keycode in SDL key events does not include Shift), m the modifiers. Keys with Ctrl, Alt
// or Super are chords (ChordKey) and special keys go through SpecialKey.
func DirectText(ch rune, m Mod) string {
	if m&(ModCtrl|ModAlt|ModSuper) != 0 || ch < 0x20 || ch == 0x7f || ch >= 0x40000000 {
		return ""
	}
	return Text(string(ch))
}

// Windows virtual-key codes used by FromVK.
const (
	vkBack   = 0x08
	vkTab    = 0x09
	vkReturn = 0x0D
	vkEscape = 0x1B
	vkSpace  = 0x20
	vkPrior  = 0x21
	vkNext   = 0x22
	vkEnd    = 0x23
	vkHome   = 0x24
	vkLeft   = 0x25
	vkUp     = 0x26
	vkRight  = 0x27
	vkDown   = 0x28
	vkInsert = 0x2D
	vkDelete = 0x2E
	vkF1     = 0x70
	vkF24    = 0x87
)

// FromVK maps a Windows virtual-key code to a Key. Keys that only type
// text (letters, digits, punctuation, the numeric keypad) return KeyNone.
func FromVK(vk uint32) Key {
	switch vk {
	case vkBack:
		return KeyBackspace
	case vkTab:
		return KeyTab
	case vkReturn:
		return KeyEnter
	case vkEscape:
		return KeyEscape
	case vkSpace:
		return KeySpace
	case vkPrior:
		return KeyPageUp
	case vkNext:
		return KeyPageDown
	case vkEnd:
		return KeyEnd
	case vkHome:
		return KeyHome
	case vkLeft:
		return KeyLeft
	case vkUp:
		return KeyUp
	case vkRight:
		return KeyRight
	case vkDown:
		return KeyDown
	case vkInsert:
		return KeyInsert
	case vkDelete:
		return KeyDelete
	}
	if vk >= vkF1 && vk <= vkF24 {
		return KeyF1 + Key(vk-vkF1)
	}
	return KeyNone
}
