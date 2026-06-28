package web

import (
	"html"
	"regexp"
	"strings"
)

var (
	ansiEscRE  = regexp.MustCompile(`\x1b\[([0-9;]*)m`)
	clearScrRE = regexp.MustCompile(`\x1b\[[0-9;]*[HJ]`)
)

// ansiToHTML converts ANSI SGR sequences to simple HTML spans for web display.
func ansiToHTML(raw string) string {
	s := clearScrRE.ReplaceAllString(raw, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	var out strings.Builder
	var state sgrState
	flush := func(text string) {
		if text == "" {
			return
		}
		escaped := html.EscapeString(text)
		escaped = strings.ReplaceAll(escaped, "\n", "<br>")
		if classes := state.classes(); classes != "" {
			out.WriteString(`<span class="`)
			out.WriteString(classes)
			out.WriteString(`">`)
			out.WriteString(escaped)
			out.WriteString(`</span>`)
		} else {
			out.WriteString(escaped)
		}
	}

	pos := 0
	matches := ansiEscRE.FindAllStringSubmatchIndex(s, -1)
	for _, m := range matches {
		if m[0] > pos {
			flush(s[pos:m[0]])
		}
		state.apply(s[m[2]:m[3]])
		pos = m[1]
	}
	flush(s[pos:])
	return out.String()
}

type sgrState struct {
	bold bool
	fg   string
}

func (s *sgrState) classes() string {
	var classes []string
	if s.bold {
		classes = append(classes, "ansi-bold")
	}
	if s.fg != "" {
		classes = append(classes, s.fg)
	}
	return strings.Join(classes, " ")
}

func (s *sgrState) apply(code string) {
	if code == "" || code == "0" {
		s.bold = false
		s.fg = ""
		return
	}
	for _, part := range strings.Split(code, ";") {
		switch part {
		case "1":
			s.bold = true
		case "22":
			s.bold = false
		case "30":
			s.fg = "ansi-fg-black"
		case "31":
			s.fg = "ansi-fg-red"
		case "32":
			s.fg = "ansi-fg-green"
		case "33":
			s.fg = "ansi-fg-yellow"
		case "34":
			s.fg = "ansi-fg-blue"
		case "35":
			s.fg = "ansi-fg-magenta"
		case "36":
			s.fg = "ansi-fg-cyan"
		case "37":
			s.fg = "ansi-fg-white"
		case "39":
			s.fg = ""
		case "90":
			s.fg = "ansi-fg-bright-black"
		case "91":
			s.fg = "ansi-fg-bright-red"
		case "92":
			s.fg = "ansi-fg-bright-green"
		case "93":
			s.fg = "ansi-fg-bright-yellow"
		case "94":
			s.fg = "ansi-fg-bright-blue"
		case "95":
			s.fg = "ansi-fg-bright-magenta"
		case "96":
			s.fg = "ansi-fg-bright-cyan"
		case "97":
			s.fg = "ansi-fg-bright-white"
		}
	}
}
