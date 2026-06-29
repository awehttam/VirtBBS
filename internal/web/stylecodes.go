package web

import (
	"html"
	"regexp"
	"strings"
)

var (
	scBoldRE      = regexp.MustCompile(`\*([^*\r\n]+)\*`)
	scItalicRE    = regexp.MustCompile(`/([^/\r\n]+)/`)
	scUnderlineRE = regexp.MustCompile(`_([^_\r\n]+)_`)
	scInverseRE   = regexp.MustCompile(`#([^#\r\n]+)#`)
)

func hasStyleCodes(raw string) bool {
	return scBoldRE.MatchString(raw) ||
		scItalicRE.MatchString(raw) ||
		scUnderlineRE.MatchString(raw) ||
		scInverseRE.MatchString(raw)
}

// styleCodesToHTML converts StyleCodes markup (*bold*, /italic/, _underline_, #inverse#) to HTML.
func styleCodesToHTML(raw string) string {
	s := strings.ReplaceAll(raw, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	var out strings.Builder
	out.WriteString(`<div class="stylecodes-body">`)
	for i, line := range lines {
		if i > 0 {
			out.WriteString("<br>")
		}
		out.WriteString(renderStyleCodeLine(line))
	}
	out.WriteString(`</div>`)
	return out.String()
}

func renderStyleCodeLine(line string) string {
	line = html.EscapeString(line)
	line = replaceStyleItalic(line)
	line = scBoldRE.ReplaceAllString(line, `<strong>$1</strong>`)
	line = scUnderlineRE.ReplaceAllString(line, `<u>$1</u>`)
	line = scInverseRE.ReplaceAllString(line, `<span class="sc-inverse">$1</span>`)
	return line
}

func replaceStyleItalic(line string) string {
	var out strings.Builder
	pos := 0
	for _, m := range scItalicRE.FindAllStringSubmatchIndex(line, -1) {
		start, end := m[0], m[1]
		if styleCodeSlashInURL(line, start) {
			continue
		}
		if end < len(line) && line[end] == ':' {
			continue
		}
		if start > pos {
			out.WriteString(line[pos:start])
		}
		out.WriteString(`<em>`)
		out.WriteString(line[m[2]:m[3]])
		out.WriteString(`</em>`)
		pos = end
	}
	out.WriteString(line[pos:])
	return out.String()
}

func styleCodeSlashInURL(line string, slash int) bool {
	if slash > 0 && line[slash-1] == ':' {
		return true
	}
	if slash > 1 && line[slash-1] == '/' && line[slash-2] == ':' {
		return true
	}
	return false
}
