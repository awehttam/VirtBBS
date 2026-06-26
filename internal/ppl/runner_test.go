package ppl

import (
	"strings"
	"testing"
)

func TestReadLineRaw_carriageReturn(t *testing.T) {
	in := strings.NewReader("2\r")
	var out strings.Builder
	got := readLineRaw(&rwPair{r: in, w: &out}, "Your choice: ", false)
	if got != "2" {
		t.Fatalf("readLineRaw with CR: got %q, want %q", got, "2")
	}
}

func TestRunSource_inputstrMenuQuit(t *testing.T) {
	src := `
BOOLEAN done
done = FALSE
WHILE NOT done
    INPUTSTR choice, "Choice: "
    IF choice = "Q" THEN
        done = TRUE
    ENDIF
WEND
END
`
	in := strings.NewReader("Q\r")
	var out strings.Builder
	env := &Environment{
		Print: func(s string) { out.WriteString(s) },
		Input: func(prompt string) string {
			return readLineRaw(&rwPair{r: in, w: &out}, prompt, false)
		},
	}
	if err := RunSource(src, env); err != nil {
		t.Fatalf("RunSource: %v", err)
	}
}

type rwPair struct {
	r *strings.Reader
	w *strings.Builder
}

func (p *rwPair) Read(b []byte) (int, error)  { return p.r.Read(b) }
func (p *rwPair) Write(b []byte) (int, error) { return p.w.Write(b) }

func TestGetStats_populatesVariables(t *testing.T) {
	env := &Environment{
		Print:               func(string) {},
		Input:               func(string) string { return "" },
		UserUploads:         3,
		UserDownloads:       7,
		UserBytesUploaded:   2048,
		UserBytesDownloaded: 4096,
		SessMsgsRead:        12,
		NewMsgsTotal:        5,
		BBSMsgTotal:         100,
	}
	if err := RunSource("GETSTATS\nEND\n", env); err != nil {
		t.Fatalf("RunSource: %v", err)
	}
	lexer := NewLexer("GETSTATS\nEND\n")
	prog, err := NewParser(lexer.Tokenize()).Parse()
	if err != nil {
		t.Fatal(err)
	}
	interp := NewInterpreter(prog, env)
	if err := interp.Run(); err != nil {
		t.Fatal(err)
	}
	if got := interp.getVar("U_UPLOADS").ToInt(); got != 3 {
		t.Fatalf("U_UPLOADS = %d, want 3", got)
	}
	if got := interp.getVar("U_KUP").ToInt(); got != 2 {
		t.Fatalf("U_KUP = %d, want 2", got)
	}
	if got := interp.getVar("U_NEWMSG").ToInt(); got != 5 {
		t.Fatalf("U_NEWMSG = %d, want 5", got)
	}
	if got := interp.getVar("BBS_MSGS").ToInt(); got != 100 {
		t.Fatalf("BBS_MSGS = %d, want 100", got)
	}
}

func TestRunSource_guessNumberGame(t *testing.T) {
	// Mirrors hello.pps option 2 — must prompt for input even when secret <> 0.
	src := `
INTEGER secret
INTEGER guess
INTEGER tries
BOOLEAN won
secret = 5
tries = 0
won = FALSE
WHILE NOT won AND tries < 5
    INPUTINT guess, "Your guess: "
    tries = tries + 1
    IF guess = secret THEN
        won = TRUE
        PRINTLN "Correct!"
    ELSEIF guess < secret THEN
        PRINTLN "Too low!"
    ELSE
        PRINTLN "Too high!"
    ENDIF
WEND
IF NOT won THEN
    PRINTLN "The number was " + STR(secret)
ENDIF
END
`
	in := strings.NewReader("3\r5\r")
	var out strings.Builder
	rw := &rwPair{r: in, w: &out}
	env := &Environment{
		Print: func(s string) { out.WriteString(s) },
		Input: func(prompt string) string {
			return readLineRaw(rw, prompt, false)
		},
	}
	if err := RunSource(src, env); err != nil {
		t.Fatalf("RunSource: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Too low!") {
		t.Fatalf("expected Too low! after first guess, got: %q", got)
	}
	if !strings.Contains(got, "Correct!") {
		t.Fatalf("expected Correct! after second guess, got: %q", got)
	}
}