package input

import "testing"

func TestSpecialKey(t *testing.T) {
	for _, c := range []struct {
		vk   uint32
		m    Mod
		want string
	}{
		{0x1B, 0, "<Esc>"},
		{0x0D, 0, "<CR>"},
		{0x09, ModShift, "<S-Tab>"},
		{0x25, ModCtrl, "<C-Left>"},
		{0x70, 0, "<F1>"},
		{0x7B, ModAlt, "<M-F12>"},
		{0x87, 0, "<F24>"},
		{0x20, 0, ""},
		{0x20, ModCtrl, "<C-Space>"},
		{0x41, 0, ""},
	} {
		if got := SpecialKey(FromVK(c.vk), c.m); got != c.want {
			t.Errorf("vk %#x mod %d: got %q want %q", c.vk, c.m, got, c.want)
		}
	}
}

func TestChordKey(t *testing.T) {
	for _, c := range []struct {
		ch   rune
		m    Mod
		want string
	}{
		{'a', ModCtrl, "<C-a>"},
		{'A', ModCtrl | ModShift, "<C-A>"},
		{'x', ModAlt, "<M-x>"},
		{'<', ModCtrl, "<C-lt>"},
		{'6', ModCtrl, "<C-^>"},
		{'^', ModCtrl, "<C-^>"},
		{'@', ModCtrl, "<C-@>"},
		{'-', ModCtrl, "<C-->"}, // as Neovide and nvim-qt send it (zoom mappings)
		{'=', ModCtrl, "<C-=>"},
		{'[', ModCtrl, "<C-[>"},
		{'\\', ModCtrl, "<C-Bslash>"},
		{'a', 0, ""},
	} {
		if got := ChordKey(c.ch, c.m); got != c.want {
			t.Errorf("%q mod %d: got %q want %q", c.ch, c.m, got, c.want)
		}
	}
}

func TestIMEWanted(t *testing.T) {
	if IMEWanted("normal", true) || !IMEWanted("insert", true) || !IMEWanted("cmdline_normal", true) || !IMEWanted("normal", false) {
		t.Fatal("IMEWanted")
	}
}
