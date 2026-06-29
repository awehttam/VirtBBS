package web

import (
	"strings"
	"testing"
)

func TestFormatMessageBodyHTML_plain(t *testing.T) {
	got := FormatMessageBodyHTML("line one\nline two")
	if !strings.Contains(got, "line one<br>line two") {
		t.Fatalf("plain newline formatting: %q", got)
	}
	if strings.Contains(got, "<span") {
		t.Fatalf("unexpected markup: %q", got)
	}
}

func TestFormatMessageBodyHTML_stylecodes(t *testing.T) {
	got := FormatMessageBodyHTML("*hello*")
	if !strings.Contains(got, "<strong>hello</strong>") {
		t.Fatalf("stylecodes path: %q", got)
	}
}

func TestFormatMessageBodyHTML_ansi(t *testing.T) {
	got := FormatMessageBodyHTML("\x1b[31mred\x1b[0m")
	if !strings.Contains(got, `class="ansi-fg-red"`) {
		t.Fatalf("ansi path: %q", got)
	}
	if !strings.Contains(got, `class="ansi-screen"`) {
		t.Fatalf("expected ansi-screen wrapper: %q", got)
	}
}

func TestFormatMessageBodyHTML_ansiPrecedence(t *testing.T) {
	got := FormatMessageBodyHTML("\x1b[1m*bold*\x1b[0m")
	if strings.Contains(got, "<strong>") {
		t.Fatalf("ANSI should take precedence over style codes: %q", got)
	}
}
