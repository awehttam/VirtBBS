package ansi

import "testing"

func TestToCP437_boxDrawing(t *testing.T) {
	line := "┌──┐ │ ├─┤ └─┘"
	want := string([]byte{0xDA, 0xC4, 0xC4, 0xBF, ' ', 0xB3, ' ', 0xC3, 0xC4, 0xB4, ' ', 0xC0, 0xC4, 0xD9})
	got := ToCP437(line)
	if got != want {
		t.Fatalf("ToCP437(%q) = %q (% x), want %q (% x)", line, got, []byte(got), want, []byte(want))
	}
}

func TestToCP437_doubleBoxDrawing(t *testing.T) {
	header := "╔═╗\n╠═╣\n╚═╝"
	want := string([]byte{0xC9, 0xCD, 0xBB, '\n', 0xCC, 0xCD, 0xB9, '\n', 0xC8, 0xCD, 0xBC})
	got := ToCP437(header)
	if got != want {
		t.Fatalf("ToCP437(%q) = %q (% x), want %q (% x)", header, got, []byte(got), want, []byte(want))
	}
}

func TestEncodeOutput_sshUTF8(t *testing.T) {
	line := "┌─────────────────────────────────────────────┐"
	if got := EncodeOutput(line, false); got != line {
		t.Fatalf("EncodeOutput(cp437=false) should pass through UTF-8, got %q", got)
	}
}