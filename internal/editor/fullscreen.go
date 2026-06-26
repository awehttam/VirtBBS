// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
// (see editor.go for full text)
//
// Change History:
//   v0.0.7  2026-06-24  Full-screen ANSI editor (SlyEdit-inspired)
// ============================================================================

package editor

// runFullScreen implements the full-screen ANSI message editor.
//
// Screen layout (24-row × 80-column terminal):
//
//	Row  1  Title bar  (reverse video: BBS name, subject, mode, line/col)
//	Row  2  Help bar   (key shortcuts — scrolls in different contexts)
//	Row  3  Top border rule
//	Rows 4-22  Text content (EDIT_ROWS = 19 visible lines)
//	Row 23  Bottom border rule
//	Row 24  Status / message line
//
// Cursor movement:
//   Arrow keys, Home/End, PgUp/PgDn
//
// Editing:
//   Printable chars — insert (insert mode) or overwrite
//   Enter          — split line at cursor
//   Backspace      — delete character before cursor (join lines at col 0)
//   Del / Ctrl+D   — delete character at cursor
//   Ctrl+W         — delete word backward
//   Ctrl+G         — delete word forward
//   Ctrl+Y         — delete entire current line → cut buffer
//   Ctrl+K / Ctrl+X  — cut current line → cut buffer (append if consecutive)
//   Ctrl+U         — paste cut buffer above current line
//   Ctrl+B         — insert blank line above cursor
//   Ins / F2       — toggle insert / overwrite mode
//
// Session:
//   Ctrl+S / F2 on special line — save
//   Ctrl+A / ESC — prompt to abort
//   Ctrl+L       — force full redraw
//   F1           — help screen

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

const (
	editRows     = 19   // visible text rows
	screenRows   = 24   // total terminal rows assumed
	screenCols   = 80   // terminal columns assumed
	textRowStart = 4    // 1-based terminal row where text editing begins
	titleRow     = 1
	helpRow      = 2
	topRuleRow   = 3
	botRuleRow   = 23
	statusRow    = 24
)

// ─── ANSI primitives ─────────────────────────────────────────────────────────

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiRevVid  = "\x1b[7m"
	ansiClear   = "\x1b[2J"
	ansiHome    = "\x1b[H"
	ansiErasEOL = "\x1b[K"

	colTitle  = "\x1b[1;44;97m"  // bold white on blue  — title bar
	colHelp   = "\x1b[1;40;96m"  // bold cyan on black  — help bar
	colRule   = "\x1b[1;36m"     // bold cyan            — border rules
	colStatus = "\x1b[1;43;30m"  // bold black on yellow — status bar
	colInfo   = "\x1b[0;33m"     // yellow               — info messages
	colErr    = "\x1b[1;31m"     // bold red             — error messages
	colLine   = "\x1b[0;97m"     // bright white         — text lines
	colCurLine= "\x1b[0;1;97m"   // bold bright white    — active line highlight
	colPrompt = "\x1b[1;32m"     // bold green           — prompts
	colQuote  = "\x1b[0;90m"     // dark grey            — quoted text
)

func moveTo(row, col int) string {
	return fmt.Sprintf("\x1b[%d;%dH", row, col)
}

func (e *fsEditor) writeStr(s string) {
	_, _ = io.WriteString(e.rw, e.enc(s))
}

// ─── Editor state ─────────────────────────────────────────────────────────────

type fsEditor struct {
	rw      io.ReadWriter
	cfg     Config
	enc     func(string) string
	lines   []string  // text buffer (each element = one line, no newlines)
	cx      int       // cursor column (0-based char index into current line)
	cy      int       // cursor row (0-based index into lines[])
	topLine int       // index of first visible line
	insert  bool      // true = insert mode; false = overwrite
	cutBuf  []string  // cut buffer — accumulates consecutive Ctrl+K lines
	lastCut bool      // true if last op was a cut (for append-cut behaviour)
	status  string    // message shown in status row (clears on next keypress)
	kr      *keyReader
}

// runFullScreen is the entry point for the full-screen editor.
func runFullScreen(rw io.ReadWriter, cfg Config) Result {
	lines := splitLines(cfg.InitBody)
	// Ensure at least one line.
	if len(lines) == 0 {
		lines = []string{""}
	}

	e := &fsEditor{
		rw:     rw,
		cfg:    cfg,
		enc:    editorEncode(cfg),
		lines:  lines,
		cx:     0,
		cy:     0,
		insert: true,
	}
	// Position cursor at end of pre-filled text.
	if cfg.InitBody != "" {
		e.cy = len(lines) - 1
		e.cx = utf8.RuneCountInString(lines[e.cy])
	}
	e.kr = newKeyReader(rw)
	defer e.kr.stop()

	e.draw()
	return e.run()
}

// ─── Main loop ────────────────────────────────────────────────────────────────

func (e *fsEditor) run() Result {
	for {
		kp := e.kr.next()

		// Clear status message on any keypress.
		hadStatus := e.status != ""
		e.status = ""

		switch kp.K {
		// ── Save ──────────────────────────────────────────────────────────────
		case keyCtrlS:
			return e.save()

		// ── Abort ─────────────────────────────────────────────────────────────
		case keyCtrlA, keyCtrlQ:
			if e.confirmAbort() {
				e.clearScreen()
				return Result{Aborted: true}
			}
			e.draw()

		case keyEsc:
			if e.confirmAbort() {
				e.clearScreen()
				return Result{Aborted: true}
			}
			e.draw()

		// ── Cursor movement ───────────────────────────────────────────────────
		case keyUp:
			e.moveCursorUp()
		case keyDown:
			e.moveCursorDown()
		case keyLeft:
			e.moveCursorLeft()
		case keyRight:
			e.moveCursorRight()
		case keyHome, keyCtrlE:
			e.cx = 0
			e.redrawCursor()
		case keyEnd:
			e.cx = utf8.RuneCountInString(e.currentLine())
			e.redrawCursor()
		case keyPageUp:
			e.pageUp()
		case keyPageDown:
			e.pageDown()

		// ── Editing ───────────────────────────────────────────────────────────
		case keyChar:
			e.insertChar(kp.R)
		case keyEnter:
			e.splitLine()
		case keyBackspace:
			e.backspace()
		case keyDelete, keyCtrlD:
			e.deleteForward()
		case keyCtrlW:
			e.deleteWordBackward()
		case keyCtrlG:
			e.deleteWordForward()
		case keyCtrlY:
			e.deleteLine()
		case keyCtrlK, keyCtrlX:
			e.cutLine()
		case keyCtrlU:
			e.pasteCutBuffer()
		case keyCtrlB:
			e.insertBlankLine()
		case keyIns, keyF2:
			e.insert = !e.insert
			e.drawTitleBar()
			e.redrawCursor()

		// ── Display ───────────────────────────────────────────────────────────
		case keyCtrlL:
			e.draw()

		// ── Help ──────────────────────────────────────────────────────────────
		case keyF1:
			e.showHelp()
			e.draw()

		default:
			if hadStatus {
				e.drawStatusRow()
			}
		}

		// Consecutive cut tracking.
		if kp.K != keyCtrlK && kp.K != keyCtrlX {
			e.lastCut = false
		}

		if e.status != "" {
			e.drawStatusRow()
		} else if hadStatus {
			e.drawStatusRow() // clear
		}
	}
}

// ─── Screen drawing ───────────────────────────────────────────────────────────

// draw performs a full redraw of the entire editor screen.
func (e *fsEditor) draw() {
	var sb strings.Builder
	sb.WriteString(ansiClear + ansiHome)
	e.buildTitleBar(&sb)
	e.buildHelpBar(&sb)
	e.buildRule(&sb, topRuleRow)
	e.buildTextArea(&sb)
	e.buildRule(&sb, botRuleRow)
	e.buildStatusRow(&sb)
	// Position cursor.
	sb.WriteString(e.cursorPos())
	e.writeStr( sb.String())
}

func (e *fsEditor) clearScreen() {
	e.writeStr( ansiClear+ansiHome+ansiReset)
}

// ── Title bar ─────────────────────────────────────────────────────────────────

func (e *fsEditor) drawTitleBar() {
	var sb strings.Builder
	e.buildTitleBar(&sb)
	e.writeStr( sb.String()+e.cursorPos())
}

func (e *fsEditor) buildTitleBar(sb *strings.Builder) {
	modeStr := "INS"
	if !e.insert {
		modeStr = "OVR"
	}
	title := fmt.Sprintf(" %s — %s", e.cfg.BBSName, e.cfg.Subject)
	lineInfo := fmt.Sprintf("Ln:%d/%d Col:%d [%s] ", e.cy+1, len(e.lines), e.cx+1, modeStr)

	pad := screenCols - utf8.RuneCountInString(title) - utf8.RuneCountInString(lineInfo)
	if pad < 1 {
		pad = 1
	}
	sb.WriteString(moveTo(titleRow, 1))
	sb.WriteString(colTitle)
	sb.WriteString(title)
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(lineInfo)
	sb.WriteString(ansiReset)
}

// ── Help bar ──────────────────────────────────────────────────────────────────

func (e *fsEditor) buildHelpBar(sb *strings.Builder) {
	help := " ^S=Save  ^A=Abort  ^K=Cut  ^U=Paste  ^B=Ins.Line  ^Y=Del.Line  ^W=Del.Word  F1=Help"
	if len(help) > screenCols {
		help = help[:screenCols]
	}
	sb.WriteString(moveTo(helpRow, 1))
	sb.WriteString(colHelp)
	sb.WriteString(help)
	sb.WriteString(strings.Repeat(" ", screenCols-utf8.RuneCountInString(help)))
	sb.WriteString(ansiReset)
}

// ── Border rules ──────────────────────────────────────────────────────────────

func (e *fsEditor) buildRule(sb *strings.Builder, row int) {
	sb.WriteString(moveTo(row, 1))
	sb.WriteString(colRule)
	sb.WriteString(strings.Repeat("─", screenCols))
	sb.WriteString(ansiReset)
}

// ── Text area ─────────────────────────────────────────────────────────────────

func (e *fsEditor) buildTextArea(sb *strings.Builder) {
	for screenIdx := 0; screenIdx < editRows; screenIdx++ {
		lineIdx := e.topLine + screenIdx
		sb.WriteString(moveTo(textRowStart+screenIdx, 1))
		if lineIdx < len(e.lines) {
			e.buildLineContent(sb, lineIdx)
		} else {
			// Empty row — show tilde like vim/nano.
			sb.WriteString("\x1b[90m~\x1b[0m")
		}
		sb.WriteString(ansiErasEOL)
	}
}

func (e *fsEditor) buildLineContent(sb *strings.Builder, lineIdx int) {
	line := e.lines[lineIdx]
	isQuote := strings.HasPrefix(line, ">") || strings.HasPrefix(line, " >")

	if isQuote {
		sb.WriteString(colQuote)
	} else if lineIdx == e.cy {
		sb.WriteString(colCurLine)
	} else {
		sb.WriteString(colLine)
	}

	// Truncate to screen width for display only.
	runes := []rune(line)
	if len(runes) > screenCols-1 {
		sb.WriteString(string(runes[:screenCols-1]))
		sb.WriteString("\x1b[90m…\x1b[0m") // ellipsis for overlong lines
	} else {
		sb.WriteString(line)
	}
	sb.WriteString(ansiReset)
}

// ── Status row ────────────────────────────────────────────────────────────────

func (e *fsEditor) drawStatusRow() {
	var sb strings.Builder
	e.buildStatusRow(&sb)
	e.writeStr( sb.String()+e.cursorPos())
}

func (e *fsEditor) buildStatusRow(sb *strings.Builder) {
	var msg string
	if e.status != "" {
		msg = e.status
	} else {
		wrapInfo := fmt.Sprintf("Wrap:%d", e.cfg.WrapCol)
		msg = fmt.Sprintf(" Lines:%d  %s  Cut:%d lines  /S=Save  /A=Abort", len(e.lines), wrapInfo, len(e.cutBuf))
	}
	if utf8.RuneCountInString(msg) > screenCols {
		runes := []rune(msg)
		msg = string(runes[:screenCols])
	}
	pad := screenCols - utf8.RuneCountInString(msg)
	sb.WriteString(moveTo(statusRow, 1))
	sb.WriteString(colStatus)
	sb.WriteString(msg)
	sb.WriteString(strings.Repeat(" ", pad))
	sb.WriteString(ansiReset)
}

// ── Cursor positioning ────────────────────────────────────────────────────────

// cursorPos returns the ANSI sequence to move to the current editor cursor.
func (e *fsEditor) cursorPos() string {
	screenRow := textRowStart + (e.cy - e.topLine)
	// Clamp col to visible area (long lines scroll horizontally… for now clamp).
	col := e.cx + 1
	if col > screenCols {
		col = screenCols
	}
	return moveTo(screenRow, col)
}

// redrawCursor repositions the terminal cursor without a full redraw.
// Also redraws the title bar so line/col stays current.
func (e *fsEditor) redrawCursor() {
	e.drawTitleBar()
	e.writeStr( e.cursorPos())
}

// redrawFromLine redraws from lineIdx down to the bottom of the text area.
func (e *fsEditor) redrawFromLine(lineIdx int) {
	var sb strings.Builder
	startScreen := lineIdx - e.topLine
	if startScreen < 0 {
		startScreen = 0
	}
	for screenIdx := startScreen; screenIdx < editRows; screenIdx++ {
		lIdx := e.topLine + screenIdx
		sb.WriteString(moveTo(textRowStart+screenIdx, 1))
		if lIdx < len(e.lines) {
			e.buildLineContent(&sb, lIdx)
		} else {
			sb.WriteString("\x1b[90m~\x1b[0m")
		}
		sb.WriteString(ansiErasEOL)
	}
	e.buildTitleBar(&sb)
	sb.WriteString(e.cursorPos())
	e.writeStr( sb.String())
}

// redrawLine redraws a single line and repositions the cursor.
func (e *fsEditor) redrawLine(lineIdx int) {
	screenIdx := lineIdx - e.topLine
	if screenIdx < 0 || screenIdx >= editRows {
		return
	}
	var sb strings.Builder
	sb.WriteString(moveTo(textRowStart+screenIdx, 1))
	e.buildLineContent(&sb, lineIdx)
	sb.WriteString(ansiErasEOL)
	e.buildTitleBar(&sb)
	sb.WriteString(e.cursorPos())
	e.writeStr( sb.String())
}

// ─── Scrolling ────────────────────────────────────────────────────────────────

// ensureCursorVisible adjusts topLine so cy is visible, then redraws if scrolled.
func (e *fsEditor) ensureCursorVisible() {
	if e.cy < e.topLine {
		e.topLine = e.cy
		e.draw()
	} else if e.cy >= e.topLine+editRows {
		e.topLine = e.cy - editRows + 1
		e.draw()
	}
}

// ─── Cursor movement ─────────────────────────────────────────────────────────

func (e *fsEditor) moveCursorUp() {
	if e.cy == 0 {
		return
	}
	e.cy--
	lineLen := utf8.RuneCountInString(e.lines[e.cy])
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.ensureCursorVisible()
	e.redrawLine(e.cy)   // re-highlight previous active line
	e.redrawLine(e.cy+1) // de-highlight old active line
}

func (e *fsEditor) moveCursorDown() {
	if e.cy >= len(e.lines)-1 {
		return
	}
	e.cy++
	lineLen := utf8.RuneCountInString(e.lines[e.cy])
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.ensureCursorVisible()
	e.redrawLine(e.cy)
	e.redrawLine(e.cy-1)
}

func (e *fsEditor) moveCursorLeft() {
	if e.cx > 0 {
		e.cx--
		e.redrawCursor()
		return
	}
	// Wrap to end of previous line.
	if e.cy > 0 {
		e.cy--
		e.cx = utf8.RuneCountInString(e.lines[e.cy])
		e.ensureCursorVisible()
		e.redrawLine(e.cy)
		e.redrawLine(e.cy+1)
	}
}

func (e *fsEditor) moveCursorRight() {
	lineLen := utf8.RuneCountInString(e.currentLine())
	if e.cx < lineLen {
		e.cx++
		e.redrawCursor()
		return
	}
	// Wrap to start of next line.
	if e.cy < len(e.lines)-1 {
		e.cy++
		e.cx = 0
		e.ensureCursorVisible()
		e.redrawLine(e.cy)
		e.redrawLine(e.cy-1)
	}
}

func (e *fsEditor) pageUp() {
	e.cy -= editRows
	if e.cy < 0 {
		e.cy = 0
	}
	e.topLine -= editRows
	if e.topLine < 0 {
		e.topLine = 0
	}
	lineLen := utf8.RuneCountInString(e.currentLine())
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.draw()
}

func (e *fsEditor) pageDown() {
	lastLine := len(e.lines) - 1
	e.cy += editRows
	if e.cy > lastLine {
		e.cy = lastLine
	}
	e.topLine += editRows
	maxTop := lastLine - editRows + 1
	if maxTop < 0 {
		maxTop = 0
	}
	if e.topLine > maxTop {
		e.topLine = maxTop
	}
	lineLen := utf8.RuneCountInString(e.currentLine())
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.draw()
}

// ─── Editing operations ───────────────────────────────────────────────────────

func (e *fsEditor) currentLine() string {
	if e.cy < len(e.lines) {
		return e.lines[e.cy]
	}
	return ""
}

func (e *fsEditor) setCurrentLine(s string) {
	for e.cy >= len(e.lines) {
		e.lines = append(e.lines, "")
	}
	e.lines[e.cy] = s
}

// insertChar inserts or overwrites a character at the cursor.
func (e *fsEditor) insertChar(r rune) {
	if len(e.lines) >= e.cfg.MaxLines && e.cy >= len(e.lines) {
		e.status = fmt.Sprintf("%s Maximum %d lines reached.", colErr, e.cfg.MaxLines)
		e.drawStatusRow()
		return
	}

	runes := []rune(e.currentLine())
	col := e.cx
	if col > len(runes) {
		col = len(runes)
	}

	if e.insert {
		// Insert mode: open a slot.
		runes = append(runes[:col], append([]rune{r}, runes[col:]...)...)
	} else {
		// Overwrite mode: replace.
		if col >= len(runes) {
			runes = append(runes, r)
		} else {
			runes[col] = r
		}
	}

	e.setCurrentLine(string(runes))
	e.cx++

	// Word wrap: if line exceeds WrapCol and we're past the wrap point,
	// find the last space and push the rest to the next line.
	if utf8.RuneCountInString(e.currentLine()) > e.cfg.WrapCol {
		e.doWordWrap()
		return
	}
	e.redrawLine(e.cy)
}

// doWordWrap splits the current line at the last space before WrapCol.
func (e *fsEditor) doWordWrap() {
	runes := []rune(e.currentLine())
	wc := e.cfg.WrapCol

	// Find last space at or before wrapCol.
	split := -1
	for i := wc - 1; i > 0; i-- {
		if i < len(runes) && runes[i] == ' ' {
			split = i
			break
		}
	}
	if split < 0 {
		// No space found — hard wrap at wrapCol.
		split = wc
	}

	before := string(runes[:split])
	after := strings.TrimLeft(string(runes[split:]), " ")

	e.setCurrentLine(before)

	// Adjust cursor: if cursor is in the wrapped portion, move it to next line.
	wrappedStart := split
	if e.cx > wrappedStart {
		// Cursor moves to next line.
		nextCX := e.cx - wrappedStart
		if nextCX < 0 {
			nextCX = 0
		}
		e.insertLineAfter(e.cy, after)
		e.cy++
		e.cx = nextCX
	} else {
		e.insertLineAfter(e.cy, after)
	}

	e.ensureCursorVisible()
	e.redrawFromLine(e.cy - 1)
}

// insertLineAfter inserts a new line with content after lineIdx.
func (e *fsEditor) insertLineAfter(lineIdx int, content string) {
	newLines := make([]string, len(e.lines)+1)
	copy(newLines, e.lines[:lineIdx+1])
	newLines[lineIdx+1] = content
	copy(newLines[lineIdx+2:], e.lines[lineIdx+1:])
	e.lines = newLines
}

// splitLine splits the current line at cx (Enter key).
func (e *fsEditor) splitLine() {
	if len(e.lines) >= e.cfg.MaxLines {
		e.status = fmt.Sprintf("%s Maximum %d lines reached.", colErr, e.cfg.MaxLines)
		return
	}
	runes := []rune(e.currentLine())
	col := e.cx
	if col > len(runes) {
		col = len(runes)
	}
	before := string(runes[:col])
	after := string(runes[col:])
	e.setCurrentLine(before)
	e.insertLineAfter(e.cy, after)
	e.cy++
	e.cx = 0
	e.ensureCursorVisible()
	e.redrawFromLine(e.cy - 1)
}

// backspace deletes the character before the cursor.
func (e *fsEditor) backspace() {
	if e.cx > 0 {
		runes := []rune(e.currentLine())
		col := e.cx
		if col > len(runes) {
			col = len(runes)
		}
		runes = append(runes[:col-1], runes[col:]...)
		e.setCurrentLine(string(runes))
		e.cx--
		e.redrawLine(e.cy)
		return
	}
	// At column 0: join with previous line.
	if e.cy == 0 {
		return
	}
	prevLen := utf8.RuneCountInString(e.lines[e.cy-1])
	e.lines[e.cy-1] = e.lines[e.cy-1] + e.currentLine()
	// Delete current line.
	e.lines = append(e.lines[:e.cy], e.lines[e.cy+1:]...)
	e.cy--
	e.cx = prevLen
	e.ensureCursorVisible()
	e.redrawFromLine(e.cy)
}

// deleteForward deletes the character at the cursor (Del key / Ctrl+D).
func (e *fsEditor) deleteForward() {
	runes := []rune(e.currentLine())
	if e.cx < len(runes) {
		runes = append(runes[:e.cx], runes[e.cx+1:]...)
		e.setCurrentLine(string(runes))
		e.redrawLine(e.cy)
		return
	}
	// At end of line: join with next line.
	if e.cy >= len(e.lines)-1 {
		return
	}
	e.lines[e.cy] = e.currentLine() + e.lines[e.cy+1]
	e.lines = append(e.lines[:e.cy+1], e.lines[e.cy+2:]...)
	e.redrawFromLine(e.cy)
}

// deleteWordBackward deletes from cursor back to the start of the previous word (Ctrl+W).
func (e *fsEditor) deleteWordBackward() {
	runes := []rune(e.currentLine())
	col := e.cx
	if col > len(runes) {
		col = len(runes)
	}
	if col == 0 {
		return
	}
	// Skip trailing spaces.
	i := col
	for i > 0 && runes[i-1] == ' ' {
		i--
	}
	// Skip word characters.
	for i > 0 && runes[i-1] != ' ' {
		i--
	}
	runes = append(runes[:i], runes[col:]...)
	e.setCurrentLine(string(runes))
	e.cx = i
	e.redrawLine(e.cy)
}

// deleteWordForward deletes from cursor to end of next word (Ctrl+G).
func (e *fsEditor) deleteWordForward() {
	runes := []rune(e.currentLine())
	col := e.cx
	if col >= len(runes) {
		return
	}
	// Skip spaces.
	i := col
	for i < len(runes) && runes[i] == ' ' {
		i++
	}
	// Skip word.
	for i < len(runes) && runes[i] != ' ' {
		i++
	}
	runes = append(runes[:col], runes[i:]...)
	e.setCurrentLine(string(runes))
	e.redrawLine(e.cy)
}

// deleteLine deletes the entire current line and adds it to the cut buffer.
func (e *fsEditor) deleteLine() {
	e.cutBuf = []string{e.currentLine()}
	if len(e.lines) == 1 {
		e.lines[0] = ""
		e.cx = 0
		e.redrawLine(0)
		return
	}
	e.lines = append(e.lines[:e.cy], e.lines[e.cy+1:]...)
	if e.cy >= len(e.lines) {
		e.cy = len(e.lines) - 1
	}
	lineLen := utf8.RuneCountInString(e.currentLine())
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.ensureCursorVisible()
	e.redrawFromLine(e.cy)
	e.status = fmt.Sprintf("%s Line deleted (^U to paste).", colInfo)
}

// cutLine cuts the current line to the cut buffer (Ctrl+K).
// Consecutive Ctrl+K appends to the buffer (like Emacs).
func (e *fsEditor) cutLine() {
	if e.lastCut {
		e.cutBuf = append(e.cutBuf, e.currentLine())
	} else {
		e.cutBuf = []string{e.currentLine()}
	}
	e.lastCut = true

	if len(e.lines) == 1 {
		e.lines[0] = ""
		e.cx = 0
		e.redrawLine(0)
		return
	}
	e.lines = append(e.lines[:e.cy], e.lines[e.cy+1:]...)
	if e.cy >= len(e.lines) {
		e.cy = len(e.lines) - 1
	}
	lineLen := utf8.RuneCountInString(e.currentLine())
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.ensureCursorVisible()
	e.redrawFromLine(e.cy)
	e.status = fmt.Sprintf("%s %d line(s) in cut buffer (^U to paste).", colInfo, len(e.cutBuf))
}

// pasteCutBuffer inserts cut buffer lines above the current line (Ctrl+U).
func (e *fsEditor) pasteCutBuffer() {
	if len(e.cutBuf) == 0 {
		e.status = colErr + " Cut buffer is empty."
		return
	}
	// Insert cut buffer lines before cy.
	newLines := make([]string, 0, len(e.lines)+len(e.cutBuf))
	newLines = append(newLines, e.lines[:e.cy]...)
	newLines = append(newLines, e.cutBuf...)
	newLines = append(newLines, e.lines[e.cy:]...)
	e.lines = newLines
	e.cy += len(e.cutBuf)
	lineLen := utf8.RuneCountInString(e.currentLine())
	if e.cx > lineLen {
		e.cx = lineLen
	}
	e.ensureCursorVisible()
	e.redrawFromLine(e.cy - len(e.cutBuf))
	e.status = fmt.Sprintf("%s Pasted %d line(s).", colInfo, len(e.cutBuf))
}

// insertBlankLine inserts a blank line above the current line (Ctrl+B).
func (e *fsEditor) insertBlankLine() {
	if len(e.lines) >= e.cfg.MaxLines {
		e.status = fmt.Sprintf("%s Maximum %d lines reached.", colErr, e.cfg.MaxLines)
		return
	}
	newLines := make([]string, len(e.lines)+1)
	copy(newLines, e.lines[:e.cy])
	newLines[e.cy] = ""
	copy(newLines[e.cy+1:], e.lines[e.cy:])
	e.lines = newLines
	e.cx = 0
	e.ensureCursorVisible()
	e.redrawFromLine(e.cy)
}

// ─── Save / Abort ─────────────────────────────────────────────────────────────

func (e *fsEditor) save() Result {
	body := joinLines(e.lines)
	e.clearScreen()
	return Result{Body: body, Lines: len(e.lines)}
}

// confirmAbort asks the user if they really want to abort.
// Returns true if they confirm.
func (e *fsEditor) confirmAbort() bool {
	// Draw prompt in status row.
	msg := colErr + " Abort message? All text will be lost. (Y/N): " + ansiReset
	e.writeStr( moveTo(statusRow, 1)+colStatus+msg+strings.Repeat(" ", 10)+ansiReset+moveTo(statusRow, 50))

	// Read single key.
	kp := e.kr.next()
	if kp.K == keyChar && (kp.R == 'Y' || kp.R == 'y') {
		return true
	}
	e.status = colInfo + " Abort cancelled."
	return false
}

// ─── Help screen ─────────────────────────────────────────────────────────────

func (e *fsEditor) showHelp() {
	var sb strings.Builder
	sb.WriteString(ansiClear + ansiHome)
	sb.WriteString(colTitle)
	sb.WriteString(fmt.Sprintf(" %-78s", "VirtBBS Full-Screen Editor — Help"))
	sb.WriteString(ansiReset + "\r\n\r\n")

	helpItems := [][2]string{
		{"Ctrl+S", "Save message and send"},
		{"Ctrl+A / ESC", "Abort (prompts for confirmation)"},
		{"F1", "This help screen"},
		{"", ""},
		{"Arrow keys", "Move cursor"},
		{"Home / End", "Start / end of line"},
		{"PgUp / PgDn", "Scroll page"},
		{"", ""},
		{"Enter", "Split line at cursor"},
		{"Backspace", "Delete character before cursor (joins lines at col 1)"},
		{"Del / Ctrl+D", "Delete character at cursor (joins lines at end)"},
		{"Ctrl+W", "Delete word backward"},
		{"Ctrl+G", "Delete word forward"},
		{"Ctrl+Y", "Delete entire line"},
		{"Ctrl+K", "Cut line (consecutive Ctrl+K appends to buffer)"},
		{"Ctrl+U", "Paste (uncut) cut buffer above current line"},
		{"Ctrl+B", "Insert blank line above cursor"},
		{"Ins / F2", "Toggle Insert / Overwrite mode"},
		{"Ctrl+L", "Force full screen redraw"},
		{"", ""},
		{"Word wrap", fmt.Sprintf("Automatic at column %d", e.cfg.WrapCol)},
		{"Max lines", fmt.Sprintf("%d", e.cfg.MaxLines)},
	}

	for _, item := range helpItems {
		if item[0] == "" {
			sb.WriteString("\r\n")
			continue
		}
		sb.WriteString(fmt.Sprintf("  \x1b[1;33m%-20s\x1b[0m %s\r\n", item[0], item[1]))
	}

	sb.WriteString("\r\n")
	sb.WriteString(colPrompt + "  Press any key to return to editor…" + ansiReset)
	e.writeStr( sb.String())
	e.kr.next() // wait for any key
}
