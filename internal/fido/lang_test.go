package fido

import (
	"strings"
	"testing"
)

func TestNormalizeLangCode(t *testing.T) {
	tests := map[string]string{
		"en": "en", "ES": "es", "af": "af", "": "en", "fr": "en",
	}
	for in, want := range tests {
		if got := NormalizeLangCode(in); got != want {
			t.Errorf("NormalizeLangCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMergeOriginKludges(t *testing.T) {
	out := MergeOriginKludges("", "af")
	if !strings.Contains(out, "\x01LANG: af") {
		t.Fatalf("missing LANG kludge: %q", out)
	}
	if !strings.Contains(out, "\x01TZUTC:") {
		t.Fatalf("missing TZUTC kludge: %q", out)
	}
	replaced := MergeOriginKludges("\x01LANG: en\r\x01TZUTC: +0000\r\x01FOO: bar", "es")
	if strings.Contains(replaced, "\x01LANG: en") {
		t.Fatalf("old LANG not replaced: %q", replaced)
	}
	if !strings.Contains(replaced, "\x01LANG: es") {
		t.Fatalf("new LANG missing: %q", replaced)
	}
	if !strings.Contains(replaced, "\x01FOO: bar") {
		t.Fatalf("other kludge dropped: %q", replaced)
	}
}
