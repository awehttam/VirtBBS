package web

import (
	"strings"
	"testing"
)

func TestStyleCodesToHTML_basic(t *testing.T) {
	html := styleCodesToHTML("*bold* and /italic/ with _underline_ and #inverse#")
	for _, want := range []string{
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<u>underline</u>",
		`<span class="sc-inverse">inverse</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("missing %q in %q", want, html)
		}
	}
}

func TestStyleCodesToHTML_preservesURLs(t *testing.T) {
	html := styleCodesToHTML("see https://example.com/path for info")
	if strings.Contains(html, "<em>") {
		t.Fatalf("should not italicize URL segments: %q", html)
	}
	if !strings.Contains(html, "https://example.com/path") {
		t.Fatalf("URL not preserved: %q", html)
	}
}

func TestHasStyleCodes(t *testing.T) {
	if !hasStyleCodes("hello *world*") {
		t.Fatal("expected style codes detected")
	}
	if hasStyleCodes("plain text only") {
		t.Fatal("expected no style codes")
	}
}
