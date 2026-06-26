package ansi

import "testing"

func TestDecodeANSBytes_cp437BoxDrawing(t *testing.T) {
	// PCBoard-native .ANS: ╔ is stored as single byte 0xC9, which Go reads as U+00C9.
	raw := string([]byte{0xC9, 0xCD, 0xBB})
	want := "╔═╗"
	if got := DecodeANSBytes(raw); got != want {
		t.Fatalf("DecodeANSBytes(% x) = %q, want %q", []byte(raw), got, want)
	}
}

func TestDecodeANSBytes_utf8Passthrough(t *testing.T) {
	line := "╔═╗ Hello"
	if got := DecodeANSBytes(line); got != line {
		t.Fatalf("DecodeANSBytes should pass through UTF-8, got %q", got)
	}
}

func TestExpandPCBAnsi_logonColors(t *testing.T) {
	in := "[1;36m║[0m [1;33m Welcome"
	want := "\x1b[1;36m║\x1b[0m \x1b[1;33m Welcome"
	if got := ExpandPCBAnsi(in); got != want {
		t.Fatalf("ExpandPCBAnsi(%q) = %q, want %q", in, got, want)
	}
}

func TestExpandPCBAnsi_menuTextUntouched(t *testing.T) {
	in := "[S]tats  [Q]uit"
	if got := ExpandPCBAnsi(in); got != in {
		t.Fatalf("ExpandPCBAnsi should not alter menu labels, got %q", got)
	}
}

func TestExpandPCBAnsi_resetCode(t *testing.T) {
	in := "[0m"
	want := "\x1b[0m"
	if got := ExpandPCBAnsi(in); got != want {
		t.Fatalf("ExpandPCBAnsi(%q) = %q, want %q", in, got, want)
	}
}