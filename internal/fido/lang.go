package fido

import (
	"fmt"
	"strings"
	"time"
)

// VirtBBS LANG kludge (experimental, not FTSC-standard):
//   ^ALANG: <ISO 639-1 code>
// Per FTS-4000, unknown ^A kludges are retained verbatim through echomail routing.
const kludgeLANG = "LANG"

// NormalizeLangCode returns a supported two-letter UI language code.
func NormalizeLangCode(code string) string {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "es", "af":
		return strings.ToLower(strings.TrimSpace(code))
	default:
		return "en"
	}
}

// LangKludgeLine returns the ^ALANG control line for a language code.
func LangKludgeLine(code string) string {
	return fmt.Sprintf("\x01%s: %s", kludgeLANG, NormalizeLangCode(code))
}

// MergeOriginKludges returns ^ALANG and ^ATZUTC kludges for locally originated mail,
// preserving any other lines from existing (one line per kludge, \r-separated).
func MergeOriginKludges(existing, lang string) string {
	langLine := LangKludgeLine(lang)
	tzLine := fmt.Sprintf("\x01TZUTC: %s", time.Now().Format("-0700"))
	var kept []string
	for _, line := range strings.Split(existing, "\r") {
		line = strings.TrimSpace(strings.TrimRight(line, "\n"))
		if line == "" {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "\x01LANG:") || strings.HasPrefix(upper, "\x01TZUTC:") {
			continue
		}
		kept = append(kept, line)
	}
	out := append(kept, langLine, tzLine)
	return strings.Join(out, "\r")
}

// ParseLangFromKludges extracts the LANG code from stored ^A kludge text, or "".
func ParseLangFromKludges(kludges string) string {
	for _, line := range strings.Split(kludges, "\r") {
		line = strings.TrimSpace(strings.TrimRight(line, "\n"))
		if !strings.HasPrefix(strings.ToUpper(line), "\x01LANG:") {
			continue
		}
		if i := strings.Index(line, ":"); i >= 0 {
			return NormalizeLangCode(strings.TrimSpace(line[i+1:]))
		}
	}
	return ""
}
