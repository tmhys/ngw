package gui

import "testing"

func TestGlyphText(t *testing.T) {
	for in, want := range map[string]string{
		"a":                                "a",
		"あ":                                "あ",
		"❤️":                               "❤",          // red heart with VS16
		"\U0001F44D\U0001F3FD":             "\U0001F44D", // thumbs up, skin tone
		"\U0001F468‍\U0001F469‍\U0001F467": "\U0001F468", // family (ZWJ)
		"1️⃣":                              "1",          // keycap
		"が":                               "が",         // か + combining voiced mark
		"é":                               "é",
	} {
		if got := glyphText(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestSolo(t *testing.T) {
	for in, want := range map[string]bool{
		"a": false, "あ": false, "": false,
		"\U000F04B2": true, "\U0001F363": true, "é": true,
	} {
		if got := solo(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
}
