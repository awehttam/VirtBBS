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
	classes := ""
	flush := func(text string) {
		if text == "" {
			return
		}
		escaped := html.EscapeString(text)
		escaped = strings.ReplaceAll(escaped, "\n", "<br>")
		if classes != "" {
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
		code := s[m[2]:m[3]]
		classes = ansiClasses(code)
		pos = m[1]
	}
	flush(s[pos:])
	return out.String()
}

func ansiClasses(code string) string {
	if code == "" || code == "0" {
		return ""
	}
	var classes []string
	for _, part := range strings.Split(code, ";") {
		switch part {
		case "1":
			classes = append(classes, "ansi-bold")
		case "30":
			classes = append(classes, "ansi-fg-black")
		case "31":
			classes = append(classes, "ansi-fg-red")
		case "32":
			classes = append(classes, "ansi-fg-green")
		case "33":
			classes = append(classes, "ansi-fg-yellow")
		case "34":
			classes = append(classes, "ansi-fg-blue")
		case "35":
			classes = append(classes, "ansi-fg-magenta")
		case "36":
			classes = append(classes, "ansi-fg-cyan")
		case "37":
			classes = append(classes, "ansi-fg-white")
		case "90":
			classes = append(classes, "ansi-fg-bright-black")
		case "91":
			classes = append(classes, "ansi-fg-bright-red")
		case "92":
			classes = append(classes, "ansi-fg-bright-green")
		case "93":
			classes = append(classes, "ansi-fg-bright-yellow")
		case "94":
			classes = append(classes, "ansi-fg-bright-blue")
		case "95":
			classes = append(classes, "ansi-fg-bright-magenta")
		case "96":
			classes = append(classes, "ansi-fg-bright-cyan")
		case "97":
			classes = append(classes, "ansi-fg-bright-white")
		}
	}
	return strings.Join(classes, " ")
}
