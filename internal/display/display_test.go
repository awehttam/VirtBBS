package display

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/virtbbs/virtbbs/internal/ansi"
)

func TestRender_logonANS_alignedWithBorder(t *testing.T) {
	content, err := os.ReadFile("../display/LOGON.ANS")
	if err != nil {
		// Fallback when test runs from package dir only.
		content = []byte(strings.Join([]string{
			"[1;36m╔══════════════════════════════════════════════╗[0m",
			"[1;36m║[0m [1;33m Welcome to @BBSNAME@                        [1;36m║[0m",
			"[1;36m║[0m [0;37m Hello, @FIRST@! Today is @DATE@ at @TIME@   [1;36m║[0m",
			"[1;36m║[0m [0;32m Security: @SECURITY@  Node: @NODE@          [1;36m║[0m",
			"[1;36m║[0m [0;35m Time limit: @TIMELEFT@ minutes remaining    [1;36m║[0m",
			"[1;36m╚══════════════════════════════════════════════╝[0m",
		}, "\n"))
	}
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/LOGON.ANS", content, 0644); err != nil {
		t.Fatal(err)
	}

	got, err := Render(dir, "LOGON", &Vars{
		BBSName:  "Larry's Farm BBS",
		Name:     "Sysop",
		Security: 110,
		Node:     7,
		TimeLeft: 60,
	})
	if err != nil {
		t.Fatal(err)
	}

	borderW := 0
	for _, line := range strings.Split(got, "\r\n") {
		if strings.ContainsRune(line, '╔') {
			for _, r := range line {
				if r == '═' {
					borderW++
				}
			}
			continue
		}
		first := strings.IndexRune(line, '║')
		last, _ := lastRuneIndex(line, '║')
		if first < 0 || last <= first {
			continue
		}
		_, firstSize := utf8.DecodeRuneInString(line[first:])
		inner := line[first+firstSize : last]
		if w := ansi.VisibleWidth(inner); w != borderW {
			t.Fatalf("row inner width %d != border %d: %q", w, borderW, line)
		}
	}
}

func TestRender_logonANS(t *testing.T) {
	content := strings.Join([]string{
		"[1;36m╔══════════════════════════════════════════════╗[0m",
		"[1;36m║[0m [1;33m Welcome to @BBSNAME@                        [1;36m║[0m",
		"[1;36m╚══════════════════════════════════════════════╝[0m",
	}, "\n")
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/LOGON.ANS", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := Render(dir, "LOGON", &Vars{BBSName: "Larry's Farm BBS"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "\x1b[1;36m╔") {
		t.Fatalf("rendered LOGON should start with cyan top border, got %q", got[:min(40, len(got))])
	}
	if !strings.Contains(got, "Larry's Farm BBS") {
		t.Fatalf("rendered LOGON missing substituted BBS name: %q", got)
	}
	if !strings.Contains(got, "╔") || !strings.Contains(got, "║") {
		t.Fatalf("rendered LOGON missing box drawing: %q", got)
	}
}

func TestRender_logonANS_nativeCP437(t *testing.T) {
	// PCBoard stores box drawing as raw CP437 bytes with PCB ANSI prefixes.
	content := string([]byte{
		'[', '1', ';', '3', '6', 'm', 0xC9,
	})
	for i := 0; i < 46; i++ {
		content += string([]byte{0xCD})
	}
	content += string([]byte{0xBB, '[', '0', 'm', '\n'})
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/LOGON.ANS", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := Render(dir, "LOGON", &Vars{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "\x1b[1;36m╔") {
		t.Fatalf("native CP437 LOGON should start with cyan top border, got %q", got[:min(40, len(got))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}