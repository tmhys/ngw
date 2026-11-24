package gui

import (
	"reflect"
	"testing"
)

func TestParseGuifont(t *testing.T) {
	for _, c := range []struct {
		in   string
		want FontSpec
	}{
		{"", FontSpec{}},
		{"Consolas:h12", FontSpec{Faces: []string{"Consolas"}, Size: 12}},
		{"BIZ_UDGothic:h10.5:b", FontSpec{Faces: []string{"BIZ UDGothic"}, Size: 10.5, Bold: true}},
		{"HackGen Console NF:h11,MS Gothic", FontSpec{Faces: []string{"HackGen Console NF", "MS Gothic"}, Size: 11}},
		{"Cascadia Mono:h12:qNONANTIALIASED:cDEFAULT", FontSpec{Faces: []string{"Cascadia Mono"}, Size: 12, Quality: "none"}},
		{"A\\,B:h9", FontSpec{Faces: []string{"A,B"}, Size: 9}},
		{"*", FontSpec{}},
	} {
		if got := ParseGuifont(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: got %+v want %+v", c.in, got, c.want)
		}
	}
}

func TestPixels(t *testing.T) {
	if pixels(12, 96) != 16 || pixels(12, 144) != 24 || pixels(10.5, 96) != 14 {
		t.Fatal("pixels")
	}
}
