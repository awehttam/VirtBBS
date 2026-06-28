// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//
// Change History:
//   v0.0.1  2026-06-24  Initial implementation
//   v0.0.2  2026-06-24  Phase 10: node registry, message pump, W/T/K commands, rw→ReadWriteCloser
//   v0.0.5  2026-06-24  Phase 12/14: door game menu, rich callers log Entry, logoff stats
//   v0.6.0  2026-06-26  Phase 0 (VirtAnd/VirtTerm): profile menu [T]okens option for
//                        self-service API token generate/revoke (internal/userapi auth)
// ============================================================================

// Package session manages a single connected user's BBS session.
package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/ansi"
	"github.com/virtbbs/virtbbs/internal/callers"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/display"
	"github.com/virtbbs/virtbbs/internal/door"
	"github.com/virtbbs/virtbbs/internal/editor"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/postname"
	"github.com/virtbbs/virtbbs/internal/ppl"
	"github.com/virtbbs/virtbbs/internal/transfer"
	"github.com/virtbbs/virtbbs/internal/users"
)

// Deps bundles all store dependencies.
type Deps struct {
	Users       *users.Store
	Messages    *messages.Store
	Nodes       *node.Store
	Callers     *callers.Log
	Files       *files.Store
	Conferences *conferences.Store
}

// Session holds all runtime state for one connected user.
type session struct {
	rw         io.ReadWriteCloser
	ctrl       *node.NodeControl
	user       *users.User
	nodeID     int
	conference int    // current conference ID
	confName   string // current conference display name
	startTime  time.Time
	remoteAddr string
	deps       Deps
	echoInput  bool // true for Telnet (server echoes); false for SSH (PTY echoes)
	cp437Out   bool // true for Telnet (SyncTerm etc.); false for SSH UTF-8 terminals

	// Idle / time limit support
	idleTimer *time.Timer // reset on each keypress; fires to close conn
	callTimer *time.Timer // fires when per-call time limit expires

	// Per-session statistics (written to callers log on logoff)
	statMsgsRead  int
	statMsgsLeft  int
	statFilesDown int
	statFilesUp   int
}

// Run drives the session from login through logoff.
// echoInput should be true for Telnet connections (server must echo input)
// and false for SSH connections (the PTY terminal handles echo).
func Run(rw io.ReadWriteCloser, remoteAddr string, deps Deps, echoInput bool) {
	s := &session{
		rw:         rw,
		startTime:  time.Now(),
		remoteAddr: remoteAddr,
		deps:       deps,
		echoInput:  echoInput,
		cp437Out:   echoInput, // Telnet → CP437; SSH/macOS Terminal → UTF-8
	}

	nodeID, err := deps.Nodes.Register()
	if err == nil {
		s.nodeID = nodeID
		// Register in-memory control so we can receive broadcasts/kicks.
		ctrl := node.RegisterControl(nodeID, func() { _ = rw.Close() })
		s.ctrl = ctrl
		defer func() {
			ctrl.Finish()
			node.UnregisterControl(nodeID)
			deps.Nodes.Unregister(nodeID)
		}()
		// Goroutine: push incoming broadcast/chat messages to terminal.
		go func() {
			for {
				select {
				case msg := <-ctrl.Messages:
					_, _ = io.WriteString(rw, ansi.EncodeOutput(msg, s.cp437Out))
				case <-ctrl.Done():
					return
				}
			}
		}()
	}

	cfg := config.Get()

	// Start idle timer — closes connection after idle_timeout_mins of inactivity.
	if cfg.Session.IdleTimeoutMins > 0 {
		d := time.Duration(cfg.Session.IdleTimeoutMins) * time.Minute
		s.idleTimer = time.AfterFunc(d, func() {
			_, _ = io.WriteString(rw, ansi.EncodeOutput("\r\n\033[1;31m*** Idle timeout — disconnecting ***\033[0m\r\n", s.cp437Out))
			_ = rw.Close()
		})
		defer s.idleTimer.Stop()
	}

	s.writeln(ansi.ClearScreen())
	s.banner()

	if !s.login() {
		s.writeln(ansi.Colorize(ansi.Red, "Login failed. Goodbye."))
		return
	}

	// Start per-call time limit.
	if cfg.Session.TimePerCallMins > 0 {
		d := time.Duration(cfg.Session.TimePerCallMins) * time.Minute
		s.callTimer = time.AfterFunc(d, func() {
			_, _ = io.WriteString(rw, ansi.EncodeOutput("\r\n\033[1;33m*** Your time for this call has expired. Goodbye! ***\033[0m\r\n", s.cp437Out))
			_ = rw.Close()
		})
		defer s.callTimer.Stop()
	}

	s.conference = 0
	s.confName = "General"
	if conf, err := deps.Conferences.Get(0); err == nil {
		s.confName = conf.Name
	}

	_ = deps.Nodes.Update(s.nodeID, node.StatusMain, "Main Menu", s.user.ID, s.user.Name, s.user.City)
	_ = deps.Users.RecordLogin(s.user.ID)
	_ = deps.Callers.Record(&callers.Entry{
		Timestamp:     time.Now(),
		UserName:      s.user.Name,
		City:          s.user.City,
		RemoteAddr:    s.remoteAddr,
		SecurityLevel: s.user.SecurityLevel,
		Node:          s.nodeID,
		Action:        "LOGIN",
	})

	// Show dynamic ANSI banner if user has ANSI enabled.
	if s.user.ANSI {
		bbsName := config.Get().BBS.Name
		s.write(ansi.Banner(bbsName))
	}

	// Show new message counts across all conferences.
	s.showNewMessages()

	// Display LOGON file if present.
	s.showDisplayFile("LOGON")

	s.mainMenu()

	s.showDisplayFile("LOGOFF")
	s.writeln("")
	s.writeln(ansi.Colorize(ansi.BrightCyan, "Thank you for calling "+config.Get().BBS.Name+"!"))
	s.writeln(ansi.Colorize(ansi.White, "Goodbye, "+s.user.Name+".\r\n"))

	// Write logoff record with session stats.
	_ = deps.Callers.Record(&callers.Entry{
		Timestamp:     time.Now(),
		UserName:      s.user.Name,
		City:          s.user.City,
		RemoteAddr:    s.remoteAddr,
		SecurityLevel: s.user.SecurityLevel,
		Node:          s.nodeID,
		Action:        "LOGOFF",
		DurationSecs:  int(time.Since(s.startTime).Seconds()),
		MsgsRead:      s.statMsgsRead,
		MsgsLeft:      s.statMsgsLeft,
		FilesDown:     s.statFilesDown,
		FilesUp:       s.statFilesUp,
	})
}

// ── Banner ────────────────────────────────────────────────────────────────────

func (s *session) banner() {
	cfg := config.Get()
	const innerW = 40 // visible chars between ║ borders
	border := ansi.Bold() + ansi.Color(ansi.BrightCyan)
	line := func(content string) string {
		content = padRight(content, innerW-2)
		pad := innerW - 2 - len(content)
		return border + "║  " + ansi.Reset() +
			ansi.Bold() + ansi.Color(ansi.BrightWhite) + content + ansi.Reset() +
			strings.Repeat(" ", pad) +
			border + "║" + ansi.Reset()
	}
	s.writeln(border + "╔" + strings.Repeat("═", innerW) + "╗" + ansi.Reset())
	s.writeln(line(cfg.BBS.Name))
	s.writeln(line("Powered by VirtBBS"))
	s.writeln(border + "╚" + strings.Repeat("═", innerW) + "╝" + ansi.Reset())
	s.writeln("")
}

// ── Login ─────────────────────────────────────────────────────────────────────

func (s *session) login() bool {
	for attempt := 0; attempt < 3; attempt++ {
		s.write(ansi.Prompt("Enter your name (or NEW): "))
		name := s.readline()
		if name == "" {
			return false
		}
		if strings.EqualFold(name, "new") {
			return s.newUser()
		}

		s.write(ansi.Prompt("Password: "))
		pass := s.readlineHidden()

		u, err := s.deps.Users.Authenticate(name, pass)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "\r\nInvalid name or password."))
			continue
		}
		s.user = u
		s.writeln(ansi.Colorize(ansi.BrightGreen, "\r\nWelcome back, "+u.Name+"!"))
		s.writeln(ansi.Color(ansi.White) + fmt.Sprintf("Last login: %s %s  Times on: %d",
			u.LastLoginDate, u.LastLoginTime, u.TimesOnline) + ansi.Reset())
		return true
	}
	return false
}

func (s *session) newUser() bool {
	s.writeln(ansi.Header("New User Registration"))
	s.write(ansi.Prompt("BBS handle (up to 25 chars): "))
	name := s.readline()
	if name == "" {
		return false
	}
	s.write(ansi.Prompt("Real name (FidoNet): "))
	realName := strings.TrimSpace(s.readline())
	if realName == "" {
		s.writeln(ansi.Colorize(ansi.Red, "Real name is required."))
		return false
	}
	s.write(ansi.Prompt("City/State: "))
	city := s.readline()
	s.write(ansi.Prompt("Choose a password: "))
	pass := s.readlineHidden()
	s.write(ansi.Prompt("\r\nConfirm password: "))
	pass2 := s.readlineHidden()
	if pass != pass2 || pass == "" {
		s.writeln(ansi.Colorize(ansi.Red, "\r\nPasswords do not match."))
		return false
	}
	u, err := s.deps.Users.RegisterNew(name, realName, city, pass, "en")
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "\r\nRegistration failed: "+err.Error()))
		return false
	}
	s.user = u
	s.writeln(ansi.Colorize(ansi.BrightGreen, "\r\nRegistration complete! Welcome, "+name+"!"))
	s.showDisplayFile("NEWUSER")
	return true
}

// ── Main Menu ─────────────────────────────────────────────────────────────────

func (s *session) mainMenu() {
	for {
		_ = s.deps.Nodes.Update(s.nodeID, node.StatusMain, "Main Menu", s.user.ID, s.user.Name, s.user.City)
		s.writeln("")
		s.writeln(ansi.Header("Main Menu — " + s.confName))
		timeLeftStr := ""
		if tl := s.timeLeft(); tl > 0 {
			timeLeftStr = fmt.Sprintf("  %s[%d min left]%s", ansi.Color(ansi.Yellow), tl, ansi.Reset())
		}
		s.writeln(timeLeftStr)
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [M]essages   [F]iles   [C]onference   [U]sers   [W]ho's online" +
			ansi.Reset())
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [T]alk       [D]oors   [P]PE   [S]tats   [R]profile [G]oodbye" +
			ansi.Reset())
		if s.user.Sysop {
			s.writeln(ansi.Color(ansi.BrightYellow) + "  [!] Sysop menu" + ansi.Reset())
		}
		s.write(ansi.Prompt("\r\nCommand: "))

		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "M":
			s.messagesMenu()
		case "F":
			s.filesMenu()
		case "C":
			s.conferenceMenu()
		case "U":
			s.userList()
		case "W":
			s.whoIsOnline()
		case "T":
			s.talkToNode()
		case "D":
			s.doorMenu()
		case "R":
			s.profileMenu()
		case "S":
			s.showStats()
		case "!":
			if s.user.Sysop {
				s.sysopMenu()
			}
		case "P":
			s.ppeMenu()
		case "G", "BYE", "QUIT", "EXIT", "":
			return
		default:
			s.writeln(ansi.Colorize(ansi.Red, "Unknown command."))
		}
	}
}

// ── Messages ──────────────────────────────────────────────────────────────────

func (s *session) messagesMenu() {
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusMessages, "Messages", s.user.ID, s.user.Name, s.user.City)
	for {
		high, _ := s.deps.Messages.HighMsgNumber(s.conference)
		s.writeln(ansi.Header(fmt.Sprintf("Messages — %s  (High: %d)", s.confName, high)))
		fidoEnabled := config.Get().Fido.Enabled
		netmailOpt := ""
		if fidoEnabled {
			netmailOpt = "   [K]NetMail"
		}
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [R]ead   [E]nter   [N]ew (since last)   [O]ffline (QWK)" + netmailOpt + "   [Q]uit" +
			ansi.Reset())
		s.write(ansi.Prompt("Message command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "R":
			s.write(ansi.Prompt("Start at message # (Enter=oldest): "))
			startStr := strings.TrimSpace(s.readline())
			start := 1
			if n, err := strconv.Atoi(startStr); err == nil {
				start = n
			}
			s.readMessages(start)
		case "E":
			s.enterMessage()
		case "N":
			lastRead := s.deps.Users.GetLastRead(s.user.ID, s.conference)
			s.readMessages(lastRead + 1)
		case "O":
			s.qwkMenu()
		case "K":
			if fidoEnabled {
				s.netmailMenu()
			}
		case "Q", "":
			return
		}
	}
}

func (s *session) readMessages(startNum int) {
	msgs, err := s.deps.Messages.ListFrom(s.conference, startNum, 20)
	if err != nil || len(msgs) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No messages."))
		return
	}
	var lastReadNum int
	for _, m := range msgs {
		s.displayMessageHeader(m)
		s.writeln(m.Body)

		lastReadNum = m.MsgNumber
		s.statMsgsRead++
		s.write(ansi.Prompt("[N]ext / [R]eply / [T]hread / [Q]uit: "))
		switch strings.ToUpper(strings.TrimSpace(s.readline())) {
		case "Q":
			if lastReadNum > 0 && s.user != nil {
				_ = s.deps.Users.SetLastRead(s.user.ID, s.conference, lastReadNum)
			}
			return
		case "R":
			s.enterReply(m)
		case "T":
			s.showThread(m)
		}
	}
	if lastReadNum > 0 && s.user != nil {
		_ = s.deps.Users.SetLastRead(s.user.ID, s.conference, lastReadNum)
	}
	s.writeln(ansi.Colorize(ansi.Yellow, "End of messages."))
}

func (s *session) displayMessageHeader(m *messages.Message) {
	label := "Msg"
	if m.ConferenceID == 0 && m.FidoOrigin != "" {
		label = "NetMail"
	}
	s.writeln("")
	s.writeln(ansi.Color(ansi.BrightCyan) + strings.Repeat("─", 60) + ansi.Reset())
	s.writeln(ansi.Color(ansi.BrightCyan) + fmt.Sprintf("%s #%-5d", label, m.MsgNumber) +
		ansi.Color(ansi.White) + fmt.Sprintf("  From: %-20s  To: %s", m.FromName, m.ToName) + ansi.Reset())
	s.writeln(ansi.Color(ansi.Yellow) + "  Subj: " + m.Subject + ansi.Reset())
	s.writeln(ansi.Color(ansi.White) + "  Date: " + m.DatePosted.Format("01-02-2006 15:04") + ansi.Reset())
	if m.FidoReply != "" {
		if parent, err := s.deps.Messages.GetByFidoMsgID(m.ConferenceID, m.FidoReply); err == nil && parent != nil {
			s.writeln(ansi.Color(ansi.White) + fmt.Sprintf("  Reply to: msg #%d", parent.MsgNumber) + ansi.Reset())
		}
	}
	if m.FidoMsgID != "" {
		if n, _ := s.deps.Messages.CountReplies(m.ConferenceID, m.FidoMsgID); n > 0 {
			repl := "replies"
			if n == 1 {
				repl = "reply"
			}
			s.writeln(ansi.Color(ansi.White) + fmt.Sprintf("  %d %s in thread", n, repl) + ansi.Reset())
		}
	}
	s.writeln(ansi.Color(ansi.BrightCyan) + strings.Repeat("─", 60) + ansi.Reset())
}

func (s *session) showThread(m *messages.Message) {
	thread, err := s.deps.Messages.FindThread(m.ConferenceID, m.MsgNumber)
	if err != nil || len(thread) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "Thread not available."))
		return
	}
	s.writeln(ansi.Header(fmt.Sprintf("Thread (%d message(s))", len(thread))))
	for _, tm := range thread {
		s.displayMessageHeader(tm)
		s.writeln(tm.Body)
	}
}

func (s *session) enterMessage() {
	s.write(ansi.Prompt("To: "))
	to := strings.TrimSpace(s.readline())
	if to == "" {
		to = "ALL"
	}
	s.write(ansi.Prompt("Subject: "))
	subj := strings.TrimSpace(s.readline())

	result := s.runEditor(subj, "")
	if result.Aborted || result.Body == "" {
		s.writeln(ansi.Colorize(ansi.Yellow, "Message aborted."))
		return
	}

	conf, err := s.deps.Conferences.Get(s.conference)
	if err != nil || conf == nil {
		s.writeln(ansi.Colorize(ansi.Red, "Conference not found."))
		return
	}
	if err := postname.ValidateEchoPost(conf, s.user); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, err.Error()))
		return
	}

	m := &messages.Message{
		ConferenceID: s.conference,
		FromName:     postname.ForConference(conf, s.user),
		ToName:       to,
		Subject:      subj,
		Status:       "A",
		Body:         result.Body,
	}
	s.applyFidoPostMeta(m, nil)
	if err := s.deps.Messages.Post(m); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error posting message: "+err.Error()))
		return
	}
	s.statMsgsLeft++
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Message #%d posted (%d lines).", m.MsgNumber, result.Lines)))
}

func (s *session) enterReply(orig *messages.Message) {
	subj := orig.Subject
	if !strings.HasPrefix(strings.ToUpper(subj), "RE:") {
		subj = "RE: " + subj
	}

	// Build quoted body for the editor.
	quoted := ""
	for _, line := range strings.Split(strings.ReplaceAll(orig.Body, "\r\n", "\n"), "\n") {
		quoted += "> " + line + "\r\n"
	}
	quoted += "\r\n"

	result := s.runEditor(subj, quoted)
	if result.Aborted || result.Body == "" {
		s.writeln(ansi.Colorize(ansi.Yellow, "Reply aborted."))
		return
	}

	conf, err := s.deps.Conferences.Get(s.conference)
	if err != nil || conf == nil {
		s.writeln(ansi.Colorize(ansi.Red, "Conference not found."))
		return
	}
	if err := postname.ValidateEchoPost(conf, s.user); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, err.Error()))
		return
	}

	m := &messages.Message{
		ConferenceID: s.conference,
		FromName:     postname.ForConference(conf, s.user),
		ToName:       orig.FromName,
		Subject:      subj,
		Status:       "A",
		Body:         result.Body,
	}
	s.applyFidoPostMeta(m, orig)
	if err := s.deps.Messages.Post(m); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error posting reply: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Reply #%d posted (%d lines).", m.MsgNumber, result.Lines)))
}

// applyFidoPostMeta assigns MSGID/REPLY kludges and echo flag for local posts.
func (s *session) applyFidoPostMeta(m *messages.Message, replyTo *messages.Message) {
	conf, err := s.deps.Conferences.Get(s.conference)
	if err != nil || conf == nil {
		return
	}
	lang := "en"
	if s.user != nil && strings.TrimSpace(s.user.Locale) != "" {
		lang = fido.NormalizeLangCode(s.user.Locale)
	}
	fido.ApplyLocalEchoMeta(m, conf, postname.EchoOrigAddr(conf), lang, replyTo)
}

// runEditor invokes the user's preferred editor and returns the result.
func (s *session) runEditor(subject, initBody string) editor.Result {
	cfg := config.Get()
	return editor.Edit(s.rw, editor.Config{
		Type:     s.user.EditorType,
		Subject:  subject,
		InitBody: initBody,
		WrapCol:  78,
		ANSI:     s.user.ANSI,
		MaxLines: 500,
		BBSName:  cfg.BBS.Name,
		CP437Out: s.cp437Out,
	})
}

// readBody is kept for legacy use (netmail compose, door interactions).
func (s *session) readBody() string {
	result := s.runEditor("", "")
	if result.Aborted {
		return ""
	}
	return result.Body
}

// ── Conferences ───────────────────────────────────────────────────────────────

func (s *session) conferenceMenu() {
	confs, err := s.deps.Conferences.List()
	if err != nil || len(confs) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No conferences available."))
		return
	}
	s.writeln(ansi.Header("Conference List"))
	for _, c := range confs {
		marker := "  "
		if c.ID == s.conference {
			marker = ansi.Color(ansi.BrightGreen) + "* " + ansi.Reset()
		}
		s.writeln(fmt.Sprintf("%s%s%-4d%s  %s",
			marker,
			ansi.Color(ansi.BrightCyan), c.ID, ansi.Reset(),
			c.Name))
	}
	s.write(ansi.Prompt("\r\nJoin conference # (Enter=stay): "))
	input := strings.TrimSpace(s.readline())
	if input == "" {
		return
	}
	id, err := strconv.Atoi(input)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid conference number."))
		return
	}
	conf, err := s.deps.Conferences.Get(id)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Conference not found."))
		return
	}
	if !conf.Public && s.user.SecurityLevel < conf.ReadSec {
		s.writeln(ansi.Colorize(ansi.Red, "Access denied."))
		return
	}
	s.conference = conf.ID
	s.confName = conf.Name
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Joined conference: "+conf.Name))
}

// ── Files ─────────────────────────────────────────────────────────────────────

func (s *session) filesMenu() {
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusFiles, "File Area", s.user.ID, s.user.Name, s.user.City)
	for {
		s.writeln(ansi.Header("File Area"))
		menu := "  [L]ist dirs   [B]rowse dir   [D]ownload   [U]pload   [S]earch"
		if s.user.Sysop {
			menu += "   [E]dit desc"
		}
		s.writeln(ansi.Color(ansi.BrightWhite) + menu + "   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("File command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "L":
			s.listDirs()
		case "B":
			s.browseDir()
		case "D":
			s.downloadFile()
		case "U":
			s.uploadFile()
		case "S":
			s.searchFiles()
		case "E":
			if s.user.Sysop {
				s.sysopEditFileDesc()
			} else {
				s.writeln(ansi.Colorize(ansi.Red, "Unknown command."))
			}
		case "Q", "":
			return
		default:
			s.writeln(ansi.Colorize(ansi.Red, "Unknown command."))
		}
	}
}

func (s *session) listDirs() {
	dirs, err := s.deps.Files.ListDirs()
	if err != nil || len(dirs) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No file directories available."))
		return
	}
	s.writeln(ansi.Header("File Directories"))
	s.writeln(ansi.Color(ansi.BrightCyan) + fmt.Sprintf("  %-4s  %-20s  %s", "ID", "Name", "Description") + ansi.Reset())
	s.writeln(ansi.Color(ansi.BrightCyan) + "  " + strings.Repeat("─", 56) + ansi.Reset())
	for _, d := range dirs {
		s.writeln(fmt.Sprintf("  %s%-4d%s  %-20s  %s",
			ansi.Color(ansi.BrightWhite), d.ID, ansi.Reset(),
			d.Name, d.Description))
	}
}

func (s *session) browseDir() {
	s.write(ansi.Prompt("Directory # : "))
	input := strings.TrimSpace(s.readline())
	id, err := strconv.ParseInt(input, 10, 64)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid directory number."))
		return
	}
	dir, err := s.deps.Files.GetDir(id)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Directory not found."))
		return
	}
	if s.user.SecurityLevel < dir.ReadSec {
		s.writeln(ansi.Colorize(ansi.Red, "Access denied."))
		return
	}
	fileList, err := s.deps.Files.ListFiles(id)
	if err != nil || len(fileList) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No files in this directory."))
		return
	}
	s.writeln(ansi.Header("Directory: " + dir.Name))
	s.writeln(ansi.Color(ansi.BrightCyan) +
		fmt.Sprintf("  %-20s %6s  %-10s  %s", "Filename", "Size", "Date", "Description") +
		ansi.Reset())
	s.writeln(ansi.Color(ansi.BrightCyan) + "  " + strings.Repeat("─", 64) + ansi.Reset())
	for _, f := range fileList {
		s.writeln(fmt.Sprintf("  %s%-20s%s %s  %-10s  %s",
			ansi.Color(ansi.BrightWhite), f.Filename, ansi.Reset(),
			files.FormatSize(f.Size),
			f.UploadDate,
			f.Description))
	}
}

func (s *session) searchFiles() {
	s.write(ansi.Prompt("Search for: "))
	query := strings.TrimSpace(s.readline())
	if query == "" {
		return
	}
	results, err := s.deps.Files.Search(query)
	if err != nil || len(results) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No files found matching '"+query+"'."))
		return
	}
	s.writeln(ansi.Header(fmt.Sprintf("Search Results: %d found", len(results))))
	for _, f := range results {
		s.writeln(fmt.Sprintf("  %s%-20s%s %s  %s",
			ansi.Color(ansi.BrightWhite), f.Filename, ansi.Reset(),
			files.FormatSize(f.Size),
			f.Description))
	}
}

func (s *session) downloadFile() {
	s.listDirs()
	s.write(ansi.Prompt("Directory # : "))
	dirInput := strings.TrimSpace(s.readline())
	dirID, err := strconv.ParseInt(dirInput, 10, 64)
	if err != nil {
		return
	}

	fileList, _ := s.deps.Files.ListFiles(dirID)
	if len(fileList) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No files available."))
		return
	}
	for _, f := range fileList {
		s.writeln(fmt.Sprintf("  %s%-20s%s %s  %s",
			ansi.Color(ansi.BrightWhite), f.Filename, ansi.Reset(),
			files.FormatSize(f.Size), f.Description))
	}

	s.write(ansi.Prompt("Filename to download: "))
	filename := strings.TrimSpace(s.readline())
	if filename == "" {
		return
	}

	// Find the file
	var chosen *files.File
	for _, f := range fileList {
		if strings.EqualFold(f.Filename, filename) {
			chosen = f
			break
		}
	}
	if chosen == nil {
		s.writeln(ansi.Colorize(ansi.Red, "File not found."))
		return
	}

	path := s.deps.Files.AbsPath(dirID, chosen.Filename)
	if _, err := os.Stat(path); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "File not available on disk."))
		return
	}

	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Sending %s (%s) via Zmodem...", chosen.Filename, files.FormatSize(chosen.Size))))
	s.writeln(ansi.Colorize(ansi.White, "Start your Zmodem receive now. Press Ctrl+X to abort."))

	_ = s.deps.Nodes.Update(s.nodeID, node.StatusFiles, "Downloading: "+chosen.Filename, s.user.ID, s.user.Name, s.user.City)

	if err := transfer.SendFile(s.rw, path); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "\r\nTransfer failed: "+err.Error()))
		return
	}

	_ = s.deps.Files.IncrementDownloads(chosen.ID)
	s.statFilesDown++
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Transfer complete!"))
}

func (s *session) uploadFile() {
	dirs, _ := s.deps.Files.ListDirs()
	uploadable := []*files.Dir{}
	for _, d := range dirs {
		if s.user.SecurityLevel >= d.UploadSec {
			uploadable = append(uploadable, d)
		}
	}
	if len(uploadable) == 0 {
		s.writeln(ansi.Colorize(ansi.Red, "You do not have upload access to any directory."))
		return
	}
	s.listDirs()
	s.write(ansi.Prompt("Upload to directory # : "))
	dirInput := strings.TrimSpace(s.readline())
	dirID, err := strconv.ParseInt(dirInput, 10, 64)
	if err != nil {
		return
	}

	dir, err := s.deps.Files.GetDir(dirID)
	if err != nil || s.user.SecurityLevel < dir.UploadSec {
		s.writeln(ansi.Colorize(ansi.Red, "Directory not found or access denied."))
		return
	}

	s.write(ansi.Prompt("File description: "))
	desc := strings.TrimSpace(s.readline())

	_ = s.deps.Files.EnsureDirPath(dirID)
	destDir := s.deps.Files.UploadDir(dirID)

	s.writeln(ansi.Colorize(ansi.BrightGreen, "Ready to receive via Zmodem. Start your upload now."))
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusFiles, "Uploading", s.user.ID, s.user.Name, s.user.City)

	receivedPath, err := transfer.ReceiveFile(s.rw, destDir)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "\r\nUpload failed: "+err.Error()))
		return
	}

	filename := receivedPath[strings.LastIndex(receivedPath, "/")+1:]
	_ = s.deps.Files.RegisterUpload(dirID, filename, desc, s.user.Name)
	var size int64
	if info, err := os.Stat(receivedPath); err == nil {
		size = info.Size()
	}
	s.recordFileUpload(size)
	s.rebuildLocalFile()
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Upload complete! Thank you, "+s.user.Name+"!"))
}

// ── User List ─────────────────────────────────────────────────────────────────

func (s *session) userList() {
	list, err := s.deps.Users.List()
	if err != nil || len(list) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No users found."))
		return
	}
	s.writeln(ansi.Header(fmt.Sprintf("User List (%d users)", len(list))))
	s.writeln(ansi.Color(ansi.BrightCyan) +
		fmt.Sprintf("  %-25s  %-24s  %s  %s", "Name", "City", "Sec", "Last On") +
		ansi.Reset())
	s.writeln(ansi.Color(ansi.BrightCyan) + "  " + strings.Repeat("─", 68) + ansi.Reset())
	for _, u := range list {
		s.writeln(fmt.Sprintf("  %s%-25s%s  %-24s  %3d  %s",
			ansi.Color(ansi.BrightWhite), u.Name, ansi.Reset(),
			u.City, u.SecurityLevel, u.LastLoginDate))
	}
}

// ── Sysop Menu ────────────────────────────────────────────────────────────────

func (s *session) sysopMenu() {
	for {
		s.writeln(ansi.Header("Sysop Menu"))
		s.writeln(ansi.Color(ansi.BrightYellow) +
			"  [N]ode list   [K]ick node   [C]onference   [L]og   [F]ido   [S]can files   [Q]uit" +
			ansi.Reset())
		s.write(ansi.Prompt("Sysop command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "N":
			s.sysopNodeList()
		case "K":
			s.sysopKickNode()
		case "C":
			s.sysopCreateConference()
		case "L":
			s.sysopCallersLog()
		case "F":
			s.sysopFidoMenu()
		case "S":
			s.sysopScanFiles()
		case "Q", "":
			return
		}
	}
}

// ── Who's online / Node chat ──────────────────────────────────────────────────

func (s *session) whoIsOnline() {
	nodes, err := s.deps.Nodes.List()
	if err != nil || len(nodes) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No other nodes active."))
		return
	}
	s.writeln(ansi.Header(fmt.Sprintf("Who's Online — %d node(s)", len(nodes))))
	s.writeln(ansi.Color(ansi.BrightCyan) +
		fmt.Sprintf("  %-6s  %-12s  %-25s  %-15s  %s", "Node", "Status", "User", "City", "Activity") +
		ansi.Reset())
	s.writeln(ansi.Color(ansi.BrightCyan) + "  " + strings.Repeat("─", 74) + ansi.Reset())
	for _, n := range nodes {
		name := n.UserName
		if name == "" {
			name = "(logging in)"
		}
		s.writeln(fmt.Sprintf("  %s%-6d%s  %-12s  %-25s  %-15s  %s",
			ansi.Color(ansi.BrightWhite), n.ID, ansi.Reset(),
			n.Status, name, n.City, n.Operation))
	}
}

func (s *session) talkToNode() {
	s.whoIsOnline()
	s.writeln("")
	s.write(ansi.Prompt("Send to node # (Enter=broadcast to all): "))
	nodeStr := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("Message: "))
	msg := strings.TrimSpace(s.readline())
	if msg == "" {
		return
	}
	if nodeStr == "" {
		node.BroadcastAll(s.user.Name, msg)
		s.writeln(ansi.Colorize(ansi.BrightGreen, "Broadcast sent to all nodes."))
	} else {
		toID, err := strconv.Atoi(nodeStr)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Invalid node number."))
			return
		}
		if err := node.ChatNode(toID, s.user.Name, msg); err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Could not send: "+err.Error()))
			return
		}
		s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Message sent to node %d.", toID)))
	}
}

func (s *session) sysopNodeList() {
	nodes, err := s.deps.Nodes.List()
	if err != nil || len(nodes) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No active nodes."))
		return
	}
	s.writeln(ansi.Header("Active Nodes"))
	for _, n := range nodes {
		s.writeln(fmt.Sprintf("  %sNode %-3d%s  %-12s  %-25s  %s",
			ansi.Color(ansi.BrightCyan), n.ID, ansi.Reset(),
			n.Status, n.UserName, n.Operation))
	}
}

func (s *session) sysopKickNode() {
	s.sysopNodeList()
	s.write(ansi.Prompt("Kick node # : "))
	input := strings.TrimSpace(s.readline())
	if input == "" {
		return
	}
	toID, err := strconv.Atoi(input)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid node number."))
		return
	}
	if toID == s.nodeID {
		s.writeln(ansi.Colorize(ansi.Red, "You cannot kick yourself."))
		return
	}
	// Send a warning message first, then kick.
	_ = node.ChatNode(toID, "SYSOP", "You have been disconnected by the sysop.")
	if err := node.KickNode(toID); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Kick failed: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Node %d has been disconnected.", toID)))
}

func (s *session) sysopCallersLog() {
	records, err := s.deps.Callers.TextRecords(30)
	if err != nil || len(records) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No callers log entries found."))
		return
	}
	unique, total, _ := s.deps.Callers.DailyStats()
	s.writeln(ansi.Header(fmt.Sprintf("Callers Log — Today: %d calls, %d unique", total, unique)))
	s.writeln(ansi.Color(ansi.BrightCyan) +
		fmt.Sprintf("  %-16s  %-25s  %-20s  %s", "Date/Time", "Name", "City", "Action") +
		ansi.Reset())
	s.writeln(ansi.Color(ansi.BrightCyan) + "  " + strings.Repeat("─", 74) + ansi.Reset())
	for _, r := range records {
		s.writeln("  " + ansi.Color(ansi.White) + r + ansi.Reset())
	}
}

func (s *session) sysopFidoMenu() {
	cfg := config.Get()
	if !cfg.Fido.Enabled {
		s.writeln(ansi.Colorize(ansi.Yellow, "FidoNet is disabled. Enable it in VirtBBS.DAT [fido] enabled=true."))
		return
	}
	for {
		s.writeln(ansi.Header(fmt.Sprintf("FidoNet — Node %s", cfg.Fido.Address)))
		s.writeln(fmt.Sprintf("  Uplink:   %s", cfg.Fido.Uplink))
		s.writeln(fmt.Sprintf("  Inbound:  %s", cfg.Fido.InboundDir))
		s.writeln(fmt.Sprintf("  Outbound: %s", cfg.Fido.OutboundDir))
		s.writeln("")
		areas := cfg.Fido.Areas
		if len(areas) > 0 {
			s.writeln(ansi.Color(ansi.BrightCyan) + "  Echo Area Mappings:" + ansi.Reset())
			for tag, confID := range areas {
				s.writeln(fmt.Sprintf("    %-30s → Conference %d", tag, confID))
			}
			s.writeln("")
		}
		s.writeln(ansi.Color(ansi.BrightYellow) +
			"  [T]oss inbound   [S]can outbound   [O] TIC file scan   [N]odelist   [L]oad nodelist now   [E]cho flags   [P]oll uplink" + ansi.Reset())
		s.writeln(ansi.Color(ansi.BrightYellow) +
			"  [I]Ping a node   [X]Trace a node   [A]reaFix   [F]ileFix   [J]oin reqs   [R]outing   [M] Rebuild maps   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("FidoNet command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "T":
			s.writeln(ansi.Colorize(ansi.White, "Tossing inbound packets (all networks)…"))
			result := fido.TossAll(&cfg.Fido, s.deps.Messages, s.deps.Conferences, cfg.Sysop.Name, s.deps.Files, cfg.Paths.Files)
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
				"Toss complete: %d packet(s), %d imported, %d skipped, %d held, %d TIC file(s).",
				result.Packets, result.Imported, result.Skipped, result.Orphaned, result.TICProcessed)))
			for _, n := range result.OrphanNotes {
				s.writeln(ansi.Colorize(ansi.Yellow, fmt.Sprintf(
					"  Held [%s]: %s from %s — %s", n.Reason, n.Subject, n.From, n.File)))
			}
			for _, e := range result.Errors {
				s.writeln(ansi.Colorize(ansi.Red, "  Error: "+e))
			}
		case "S":
			s.writeln(ansi.Colorize(ansi.White, "Scanning echo areas for outbound messages…"))
			result, err := fido.ScanAll(&cfg.Fido, s.deps.Messages, s.deps.Conferences, cfg.BBS.Name)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Scan error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
				"Scan complete: %d message(s) in %d PKT file(s) exported.", result.Scanned, result.PKTFiles)))
			for _, e := range result.Errors {
				s.writeln(ansi.Colorize(ansi.Red, "  Error: "+e))
			}
		case "O":
			s.writeln(ansi.Colorize(ansi.White, "Scanning file areas for outbound TIC…"))
			result, err := fido.FileScanAll(&cfg.Fido, s.deps.Messages.DB(), config.Get().Paths.Files)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "File scan error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
				"File scan complete: %d file(s) in %d TIC ticket(s).", result.Files, result.TICFiles)))
			for _, e := range result.Errors {
				s.writeln(ansi.Colorize(ansi.Red, "  Error: "+e))
			}
		case "N":
			s.nodelistBrowser()
		case "L":
			s.fidoLoadNodelist()
		case "E":
			s.echoFlagConference()
		case "P":
			s.fidoPoll()
		case "I":
			s.fidoPing()
		case "X":
			s.fidoTrace()
		case "A":
			s.fidoAreaFixMenu()
		case "F":
			s.fidoFileFixMenu()
		case "J":
			s.fidoJoinRequestsMenu()
		case "R":
			s.fidoRoutingTableMenu()
		case "M":
			s.fidoRebuildNetworkMaps()
		case "Q", "":
			return
		}
	}
}

// fidoJoinRequestsMenu reviews pending applications to join a network this
// BBS hosts (NetworkDef.IsHub()) and approves/denies them — see
// internal/fido/members.go. Approving allocates a net/node address,
// authorizes BinkP via the same Downlink mechanism AreaFix already uses,
// and immediately regenerates the nodelist.
func (s *session) fidoJoinRequestsMenu() {
	target := s.pickHubNetwork("administer join requests for")
	if target == nil {
		return
	}
	mdb := fido.OpenMembersDB(s.deps.Messages.DB())
	for {
		pending, err := mdb.ListPending(target.Name)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
			return
		}
		s.writeln(ansi.Header("Join Requests — " + target.Name))
		if len(pending) == 0 {
			s.writeln(ansi.Color(ansi.Yellow) + "  No pending requests." + ansi.Reset())
		} else {
			for _, r := range pending {
				netStr := "sysop's choice"
				if r.RequestedNet != nil {
					netStr = fmt.Sprintf("net %d", *r.RequestedNet)
				}
				s.writeln(fmt.Sprintf("  #%d  %-20s  %-20s  %s", r.ID, r.BBSName, r.SysopName, netStr))
			}
		}
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightYellow) + "  [A]pprove   [D]eny   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("Command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "A":
			s.fidoApproveJoinRequest(target, mdb)
		case "D":
			s.write(ansi.Prompt("Request # to deny: "))
			if id, err := strconv.ParseInt(strings.TrimSpace(s.readline()), 10, 64); err == nil {
				_ = mdb.Deny(id, s.user.Name)
			}
		case "Q", "":
			return
		}
	}
}

func (s *session) fidoApproveJoinRequest(target *fido.NetworkDef, mdb *fido.MembersDB) {
	s.write(ansi.Prompt("Request # to approve: "))
	id, err := strconv.ParseInt(strings.TrimSpace(s.readline()), 10, 64)
	if err != nil {
		return
	}
	req, err := mdb.GetJoinRequest(id)
	if err != nil || req == nil || req.Status != "pending" {
		s.writeln(ansi.Colorize(ansi.Red, "Request not found or already decided."))
		return
	}

	net := 1
	if req.RequestedNet != nil {
		net = *req.RequestedNet
	}
	s.write(ansi.Prompt(fmt.Sprintf("Net number (Enter=%d): ", net)))
	if numStr := strings.TrimSpace(s.readline()); numStr != "" {
		if n, err := strconv.Atoi(numStr); err == nil {
			net = n
		}
	}

	hasMembers, err := mdb.NetHasMembers(target.Name, net)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
		return
	}
	isHost := false
	node := 0
	if !hasMembers {
		s.write(ansi.Prompt(fmt.Sprintf("Net %d has no members yet — make this the net's Host (node 0)? [y/N]: ", net)))
		isHost = strings.EqualFold(strings.TrimSpace(s.readline()), "y")
	}
	if !isHost {
		node, err = mdb.NextNodeNum(target.Name, net)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
			return
		}
		s.write(ansi.Prompt(fmt.Sprintf("Node number (Enter=%d): ", node)))
		if numStr := strings.TrimSpace(s.readline()); numStr != "" {
			if n, err := strconv.Atoi(numStr); err == nil {
				node = n
			}
		}
	}

	password := randomPassword()
	saveDownlink := func(networkName string, dl fido.Downlink) error {
		return s.updateNetworkDownlinks(networkName, func(cur []fido.Downlink) []fido.Downlink {
			return append(append([]fido.Downlink{}, cur...), dl)
		})
	}

	m, err := mdb.ApproveJoinRequest(target, req, net, node, isHost, password, saveDownlink)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
		return
	}

	cfg := config.Get()
	if err := fido.ApplyNodeAnnounceInfo(target, s.deps.Messages.DB(), s.deps.Conferences, s.deps.Messages,
		&fido.Member{Network: m.Network, Zone: m.Zone, Net: m.Net, NodeNum: m.NodeNum, Point: m.Point,
			BBSName: m.BBSName, SysopName: m.SysopName, Location: m.Location, Contact: m.Contact,
			BinkpHost: m.BinkpHost, IsActive: true}, "NEW"); err != nil {
		s.writeln(ansi.Colorize(ansi.Yellow, "Member approved but welcome announcement failed: "+err.Error()))
	}
	if _, _, err := fido.GenerateNodelist(s.deps.Messages.DB(), target, cfg.BBS.Name, cfg.Sysop.Name); err != nil {
		s.writeln(ansi.Colorize(ansi.Yellow, "Member approved but nodelist regeneration failed: "+err.Error()))
	}

	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"Approved as %s. Password: %s (shown once — give this to the applicant).", m.Addr4D(), password)))

	if req.RequestedByUserID > 0 {
		_ = s.deps.Messages.Post(&messages.Message{
			ConferenceID: 0,
			FromName:     "SysOp",
			ToName:       req.SysopName,
			Subject:      fmt.Sprintf("VirtNet application approved: %s", m.Addr4D()),
			Status:       "A",
			Body: fmt.Sprintf("Your application to join %s was approved.\r\n\r\nYour address: %s\r\nYour password: %s\r\n",
				target.Name, m.Addr4D(), password),
		})
	}
}

// fidoRoutingTableMenu lists every approved member (the routing table) and
// offers BinkleyTerm-style plain-text export/import — see
// internal/fido/routingtable.go.
func (s *session) fidoRoutingTableMenu() {
	target := s.pickHubNetwork("view the routing table for")
	if target == nil {
		return
	}
	mdb := fido.OpenMembersDB(s.deps.Messages.DB())
	for {
		members, err := mdb.ListMembers(target.Name)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
			return
		}
		s.writeln(ansi.Header("Routing Table — " + target.Name))
		s.writeln(ansi.Color(ansi.BrightCyan) +
			fmt.Sprintf("  %-16s %-24s %-30s %s", "Address", "BBS", "Host:Port", "Active") + ansi.Reset())
		for _, m := range members {
			active := "Yes"
			if !m.IsActive {
				active = "No"
			}
			s.writeln(fmt.Sprintf("  %-16s %-24s %-30s %s", m.Addr4D(), m.BBSName, m.BinkpHost, active))
		}
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightYellow) + "  [X]port   [I]mport   [E]dit member   [V]iew routes   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("Command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "X":
			s.write(ansi.Prompt("Export to path: "))
			path := strings.TrimSpace(s.readline())
			if path == "" {
				continue
			}
			data, err := fido.ExportRoutingTable(s.deps.Messages.DB(), target.Name)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error writing file: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, "Exported to "+path))
		case "I":
			s.write(ansi.Prompt("Import from path: "))
			path := strings.TrimSpace(s.readline())
			if path == "" {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error reading file: "+err.Error()))
				continue
			}
			result, err := fido.ImportRoutingTable(s.deps.Messages.DB(), target.Name, data)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Updated %d. Unknown: %d.", result.Updated, len(result.Unknown))))
			for _, u := range result.Unknown {
				s.writeln(ansi.Colorize(ansi.Yellow, "  Unknown address: "+u))
			}
			for _, e := range result.Errors {
				s.writeln(ansi.Colorize(ansi.Red, "  "+e))
			}
			cfg := config.Get()
			if _, _, err := fido.GenerateNodelist(s.deps.Messages.DB(), target, cfg.BBS.Name, cfg.Sysop.Name); err != nil {
				s.writeln(ansi.Colorize(ansi.Yellow, "Nodelist regeneration failed: "+err.Error()))
			}
		case "E":
			s.fidoEditMemberInfo(target, mdb)
		case "V":
			s.fidoRoutesMenu(target)
		case "Q", "":
			return
		}
	}
}

// fidoRoutesMenu manages the ROUTES.BBS-style static routing table —
// wildcard address patterns mapped to a "route via this address instead"
// next-hop, distinct from the host:port/password table in
// fidoRoutingTableMenu. Default net->Host (/0) routes appear here
// auto-seeded (is_default=Yes) the moment a net gets a Host; the sysop can
// add/remove explicit overrides and import/export the literal ROUTES.BBS
// filename, matching real BinkleyTerm/FrontDoor convention.
func (s *session) fidoRoutesMenu(target *fido.NetworkDef) {
	for {
		routes, err := fido.ListRoutes(s.deps.Messages.DB(), target.Name)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
			return
		}
		s.writeln(ansi.Header("Routing Table (ROUTES.BBS) — " + target.Name))
		s.writeln(ansi.Color(ansi.BrightCyan) +
			fmt.Sprintf("  %-20s %-20s %s", "Pattern", "Route-to", "Default") + ansi.Reset())
		for _, r := range routes {
			isDefault := "No"
			if r.IsDefault {
				isDefault = "Yes"
			}
			s.writeln(fmt.Sprintf("  %-20s %-20s %s", r.Pattern, r.RouteTo, isDefault))
		}
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightYellow) + "  [+]Add   [-]Remove   [X]port ROUTES.BBS   [I]mport ROUTES.BBS   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("Command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "+":
			s.write(ansi.Prompt("Pattern (e.g. 300:1005/*, 300:*, or *): "))
			pattern := strings.TrimSpace(s.readline())
			s.write(ansi.Prompt("Route to (zone:net/node): "))
			routeTo := strings.TrimSpace(s.readline())
			if pattern == "" || routeTo == "" {
				continue
			}
			if err := fido.AddRoute(s.deps.Messages.DB(), target.Name, pattern, routeTo); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, "Route added."))
		case "-":
			s.write(ansi.Prompt("Pattern to remove: "))
			pattern := strings.TrimSpace(s.readline())
			if pattern == "" {
				continue
			}
			if err := fido.RemoveRoute(s.deps.Messages.DB(), target.Name, pattern); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, "Route removed."))
		case "X":
			s.write(ansi.Prompt("Export ROUTES.BBS to directory: "))
			dir := strings.TrimSpace(s.readline())
			if dir == "" {
				continue
			}
			data, err := fido.ExportRoutesBBS(s.deps.Messages.DB(), target.Name)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			path := strings.TrimRight(dir, "/") + "/ROUTES.BBS"
			if err := os.WriteFile(path, data, 0644); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error writing file: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, "Exported to "+path))
		case "I":
			s.write(ansi.Prompt("Import ROUTES.BBS from directory: "))
			dir := strings.TrimSpace(s.readline())
			if dir == "" {
				continue
			}
			path := strings.TrimRight(dir, "/") + "/ROUTES.BBS"
			data, err := os.ReadFile(path)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error reading file: "+err.Error()))
				continue
			}
			result, err := fido.ImportRoutesBBS(s.deps.Messages.DB(), target.Name, data)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Added/updated %d route(s).", result.Added)))
			for _, e := range result.Errors {
				s.writeln(ansi.Colorize(ansi.Red, "  "+e))
			}
		case "Q", "":
			return
		}
	}
}

// fidoEditMemberInfo lets the sysop edit an existing member's contact/
// location/binkp-host info — the same "edit my node info" flow described
// for a sub-hub's own sysop, since this is the only such UI in the system
// and a sub-hub is just a VirtBBS instance running this same code against
// its own net. On save it goes through ApplyNodeAnnounceInfo exactly like
// approval/inbound announcements do, and (if target.Uplink != "", i.e.
// this instance is itself a delegated sub-hub, not the real top-level hub)
// also sends a CHANGE NodeAnnounce upstream.
func (s *session) fidoEditMemberInfo(target *fido.NetworkDef, mdb *fido.MembersDB) {
	s.write(ansi.Prompt("Address to edit (zone:net/node): "))
	addrStr := strings.TrimSpace(s.readline())
	addr, err := fido.ParseAddr(addrStr)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid address: "+err.Error()))
		return
	}
	m, err := mdb.GetMemberByAddr(target.Name, addr)
	if err != nil || m == nil {
		s.writeln(ansi.Colorize(ansi.Red, "Member not found."))
		return
	}

	s.write(ansi.Prompt(fmt.Sprintf("Location (Enter=%q): ", m.Location)))
	if v := strings.TrimSpace(s.readline()); v != "" {
		m.Location = v
	}
	s.write(ansi.Prompt(fmt.Sprintf("Contact (Enter=%q): ", m.Contact)))
	if v := strings.TrimSpace(s.readline()); v != "" {
		m.Contact = v
	}
	s.write(ansi.Prompt(fmt.Sprintf("BinkP host:port (Enter=%q): ", m.BinkpHost)))
	if v := strings.TrimSpace(s.readline()); v != "" {
		m.BinkpHost = v
	}

	if err := fido.ApplyNodeAnnounceInfo(target, s.deps.Messages.DB(), s.deps.Conferences, s.deps.Messages, m, "CHANGE"); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
		return
	}
	if target.Uplink != "" {
		if err := fido.SendNodeAnnounce(target, m, "CHANGE"); err != nil {
			s.writeln(ansi.Colorize(ansi.Yellow, "Saved locally, but failed to announce upstream: "+err.Error()))
			return
		}
	}
	cfg := config.Get()
	if _, _, err := fido.GenerateNodelist(s.deps.Messages.DB(), target, cfg.BBS.Name, cfg.Sysop.Name); err != nil {
		s.writeln(ansi.Colorize(ansi.Yellow, "Nodelist regeneration failed: "+err.Error()))
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Member info updated."))
}

// fidoRebuildNetworkMaps regenerates VirtDiag.zip for a hosted hub network.
func (s *session) fidoRebuildNetworkMaps() {
	target := s.pickHubNetwork("rebuild network maps for")
	if target == nil {
		return
	}
	cfg := config.Get()
	s.writeln(ansi.Colorize(ansi.White, fmt.Sprintf("Rebuilding network maps for %s…", target.Name)))
	count, warns := fido.RebuildNetworkDiagrams(target, s.deps.Messages.DB(), s.deps.Files, cfg.BBS.Name, cfg.Sysop.Name)
	if count == 0 && len(warns) > 0 {
		s.writeln(ansi.Colorize(ansi.Red, strings.Join(warns, "; ")))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"Network maps rebuilt: %d diagram(s) written to VirtDiag.zip.", count)))
	for _, w := range warns {
		s.writeln(ansi.Colorize(ansi.Yellow, "  Warning: "+w))
	}
}

// pickHubNetwork is pickFidoNetwork, restricted to networks this BBS hosts
// (NetworkDef.IsHub()) — the only ones with join requests/a routing table.
func (s *session) pickHubNetwork(verb string) *fido.NetworkDef {
	cfg := config.Get()
	var hubs []fido.NetworkDef
	for _, nd := range cfg.Fido.AllNetworks() {
		if nd.Enabled && nd.IsHub() {
			hubs = append(hubs, nd)
		}
	}
	if len(hubs) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "This BBS doesn't host any network."))
		return nil
	}
	target := hubs[0]
	if len(hubs) > 1 {
		for i, n := range hubs {
			s.writeln(fmt.Sprintf("  %d. %s (%s)", i+1, n.Name, n.Address))
		}
		s.write(ansi.Prompt(fmt.Sprintf("Network # to %s (Enter=1): ", verb)))
		if num, err := strconv.Atoi(strings.TrimSpace(s.readline())); err == nil && num >= 1 && num <= len(hubs) {
			target = hubs[num-1]
		}
	}
	return &target
}

// randomPassword generates a short random hex password for a newly
// approved VirtNet member, mirroring the codebase's existing
// crypto/rand-based token-generation idiom (users.Store.CreateAPIToken).
func randomPassword() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "changeme"
	}
	return hex.EncodeToString(buf)
}

// fidoAreaFixMenu manages AreaFix downlinks (systems that subscribe to
// echomail areas from us) and lets the sysop request areas from our own
// uplink as a downlink — see internal/fido/areafix.go.
func (s *session) fidoAreaFixMenu() {
	target := s.pickFidoNetwork("administer")
	if target == nil {
		return
	}
	for {
		areafixDB := fido.OpenAreaFixDB(s.deps.Messages.DB())

		s.writeln(ansi.Header("AreaFix Administration — " + target.Name))
		if len(target.Downlinks) == 0 {
			s.writeln(ansi.Color(ansi.Yellow) + "  No downlinks configured." + ansi.Reset())
		} else {
			s.writeln(ansi.Color(ansi.BrightCyan) +
				fmt.Sprintf("  %-20s %-16s %s", "Name", "Address", "Subscriptions") + ansi.Reset())
			for _, dl := range target.Downlinks {
				tags, _ := areafixDB.SubscriptionsFor(target.Name, dl.Address)
				s.writeln(fmt.Sprintf("  %-20s %-16s %s", dl.Name, dl.Address, strings.Join(tags, ", ")))
			}
		}
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightYellow) +
			"  [D]ownlink add   [R]emove downlink   [U]pstream request   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("AreaFix command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "D":
			s.fidoAddDownlink(target.Name)
		case "R":
			s.fidoRemoveDownlink(target.Name)
		case "U":
			s.fidoRequestUpstreamAreas(target)
		case "Q", "":
			return
		}
		// Re-fetch in case config.Save changed it underneath us.
		if t := config.Get().Fido.NetworkByName(target.Name); t != nil {
			target = t
		}
	}
}

// fidoAddDownlink prompts for a new downlink's details and persists it to
// VirtBBS.DAT, under the given network.
func (s *session) fidoAddDownlink(networkName string) {
	s.write(ansi.Prompt("Downlink name: "))
	name := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("Downlink address (zone:net/node): "))
	addr := strings.TrimSpace(s.readline())
	if addr == "" {
		return
	}
	if _, err := fido.ParseAddr(addr); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid address: "+err.Error()))
		return
	}
	s.write(ansi.Prompt("Password the downlink must supply (blank = none): "))
	password := strings.TrimSpace(s.readline())

	dl := fido.Downlink{Name: name, Address: addr, Password: password}
	if err := s.updateNetworkDownlinks(networkName, func(cur []fido.Downlink) []fido.Downlink {
		return append(append([]fido.Downlink{}, cur...), dl)
	}); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error saving config: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Downlink %s (%s) added to %s.", name, addr, networkName)))
}

// fidoRemoveDownlink prompts for an address and removes that downlink from
// VirtBBS.DAT under the given network, also deleting its AreaFix subscriptions.
func (s *session) fidoRemoveDownlink(networkName string) {
	s.write(ansi.Prompt("Downlink address to remove: "))
	addr := strings.TrimSpace(s.readline())
	if addr == "" {
		return
	}

	removed := false
	err := s.updateNetworkDownlinks(networkName, func(cur []fido.Downlink) []fido.Downlink {
		var kept []fido.Downlink
		for _, dl := range cur {
			if strings.EqualFold(dl.Address, addr) {
				removed = true
				continue
			}
			kept = append(kept, dl)
		}
		return kept
	})
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error saving config: "+err.Error()))
		return
	}
	if !removed {
		s.writeln(ansi.Colorize(ansi.Yellow, "No downlink found with that address."))
		return
	}

	areafixDB := fido.OpenAreaFixDB(s.deps.Messages.DB())
	tags, _ := areafixDB.SubscriptionsFor(networkName, addr)
	for _, tag := range tags {
		_ = areafixDB.Unsubscribe(networkName, addr, tag)
	}
	filefixDB := fido.OpenFileFixDB(s.deps.Messages.DB())
	ftags, _ := filefixDB.SubscriptionsFor(networkName, addr)
	for _, tag := range ftags {
		_ = filefixDB.Unsubscribe(networkName, addr, tag)
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Downlink removed and AreaFix/FileFix subscriptions cleared."))
}

// updateNetworkDownlinks loads the live config, applies mutate to the named
// network's Downlinks slice (primary network if networkName matches the
// primary, otherwise the matching [[fido.networks]] entry), and saves the
// result. Returns an error if the network can't be found.
func (s *session) updateNetworkDownlinks(networkName string, mutate func([]fido.Downlink) []fido.Downlink) error {
	cfg := config.Get()
	merged := *cfg
	if strings.EqualFold(networkName, fido.PrimaryNetworkName) {
		merged.Fido.Downlinks = mutate(cfg.Fido.Downlinks)
		return config.Save(&merged)
	}
	merged.Fido.Networks = append([]fido.NetworkDef{}, cfg.Fido.Networks...)
	for i := range merged.Fido.Networks {
		if strings.EqualFold(merged.Fido.Networks[i].Name, networkName) {
			merged.Fido.Networks[i].Downlinks = mutate(merged.Fido.Networks[i].Downlinks)
			return config.Save(&merged)
		}
	}
	return fmt.Errorf("network %q not found", networkName)
}

// fidoRequestUpstreamAreas sends an AreaFix subscribe/unsubscribe request
// to nd's own uplink, so VirtBBS can act as a downlink of a larger hub.
func (s *session) fidoRequestUpstreamAreas(nd *fido.NetworkDef) {
	s.write(ansi.Prompt("Area tags to subscribe, space-separated (blank = none): "))
	addLine := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("Area tags to unsubscribe, space-separated (blank = none): "))
	removeLine := strings.TrimSpace(s.readline())
	if addLine == "" && removeLine == "" {
		return
	}

	pktPath, err := fido.RequestAreaFix(nd, s.user.Name,
		strings.Fields(addLine), strings.Fields(removeLine))
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "AreaFix request error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "AreaFix request sent → "+pktPath))
}

// fidoFileFixMenu shows FileFix file-area subscriptions for the current
// network's downlinks (the same [[fido.downlinks]] list AreaFix uses — see
// internal/fido/filefix.go) and lets the sysop request file areas from our
// own uplink's FileFix. Adding/removing downlinks themselves is done via
// the AreaFix menu, since it's the same underlying link relationship.
func (s *session) fidoFileFixMenu() {
	target := s.pickFidoNetwork("administer")
	if target == nil {
		return
	}
	for {
		filefixDB := fido.OpenFileFixDB(s.deps.Messages.DB())

		s.writeln(ansi.Header("FileFix Administration — " + target.Name))
		if len(target.Downlinks) == 0 {
			s.writeln(ansi.Color(ansi.Yellow) + "  No downlinks configured (add via AreaFix menu)." + ansi.Reset())
		} else {
			s.writeln(ansi.Color(ansi.BrightCyan) +
				fmt.Sprintf("  %-20s %-16s %s", "Name", "Address", "File subscriptions") + ansi.Reset())
			for _, dl := range target.Downlinks {
				tags, _ := filefixDB.SubscriptionsFor(target.Name, dl.Address)
				s.writeln(fmt.Sprintf("  %-20s %-16s %s", dl.Name, dl.Address, strings.Join(tags, ", ")))
			}
		}
		s.writeln(ansi.Color(ansi.BrightYellow) + "  [U]pstream request   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("FileFix command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "U":
			s.fidoRequestUpstreamFiles(target)
		case "Q", "":
			return
		}
	}
}

// fidoRequestUpstreamFiles sends a FileFix subscribe/unsubscribe request to
// nd's own uplink, so VirtBBS can act as a downlink of a larger hub for
// file areas, mirroring fidoRequestUpstreamAreas.
func (s *session) fidoRequestUpstreamFiles(nd *fido.NetworkDef) {
	s.write(ansi.Prompt("File area tags to subscribe, space-separated (blank = none): "))
	addLine := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("File area tags to unsubscribe, space-separated (blank = none): "))
	removeLine := strings.TrimSpace(s.readline())
	if addLine == "" && removeLine == "" {
		return
	}

	pktPath, err := fido.RequestFileFix(nd, s.user.Name,
		strings.Fields(addLine), strings.Fields(removeLine))
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "FileFix request error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "FileFix request sent → "+pktPath))
}

// fidoPing prompts for a FidoNet address and sends a PING test netmail to
// it. The receiving system (if it implements the same convention) replies
// automatically with PONG — see internal/fido/ping.go.
func (s *session) fidoPing() {
	target := s.pickFidoNetwork("ping from")
	if target == nil {
		return
	}
	s.write(ansi.Prompt("Ping address (zone:net/node): "))
	addr := strings.TrimSpace(s.readline())
	if addr == "" {
		return
	}

	toName := "Sysop"
	if a, err := fido.ParseAddr(addr); err == nil {
		ndb := fido.OpenNodelistDB(s.deps.Messages.DB())
		if node, err := ndb.LookupAddr(target.Name, a); err == nil && node != nil {
			toName = node.Sysop
		}
	}

	pktPath, err := fido.SendPing(target, s.user.Name, toName, addr)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Ping error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("PING sent to %s → %s", addr, pktPath)))
}

// fidoTrace prompts for a FidoNet address and sends a TRACE test netmail to
// it, mirroring fidoPing. The receiving system (if it implements the same
// convention) replies automatically with routing details — see
// internal/fido/trace.go.
func (s *session) fidoTrace() {
	target := s.pickFidoNetwork("trace from")
	if target == nil {
		return
	}
	s.write(ansi.Prompt("Trace address (zone:net/node): "))
	addr := strings.TrimSpace(s.readline())
	if addr == "" {
		return
	}

	toName := "Sysop"
	if a, err := fido.ParseAddr(addr); err == nil {
		ndb := fido.OpenNodelistDB(s.deps.Messages.DB())
		if node, err := ndb.LookupAddr(target.Name, a); err == nil && node != nil {
			toName = node.Sysop
		}
	}

	pktPath, err := fido.SendTrace(target, s.user.Name, toName, addr)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Trace error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("TRACE sent to %s → %s", addr, pktPath)))
}

// ── FidoNet in-BBS functions ──────────────────────────────────────────────────

// nodelistBrowser displays a paged nodelist browser with search.
func (s *session) nodelistBrowser() {
	cfg := config.Get()
	ndb := fido.OpenNodelistDB(s.deps.Messages.DB())
	network := "FidoNet"
	page := 1
	query := ""

	for {
		var res *fido.SearchResult
		var err error
		if query == "" {
			res, err = ndb.Search(network, "", page, 20)
		} else {
			res, err = ndb.Search(network, query, page, 20)
		}
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Nodelist error: "+err.Error()))
			return
		}

		s.writeln(ansi.Header(fmt.Sprintf("Nodelist — %s  Page %d/%d  (%d nodes)", network, page, res.Pages, res.Total)))
		if query != "" {
			s.writeln(ansi.Color(ansi.Yellow) + fmt.Sprintf("  Search: %q", query) + ansi.Reset())
		}
		s.writeln(ansi.Color(ansi.BrightCyan) +
			fmt.Sprintf("  %-18s %-25s %-20s %s", "Address", "Sysop", "Location", "BBS Name") +
			ansi.Reset())
		s.writeln(ansi.Color(ansi.BrightCyan) + strings.Repeat("─", 78) + ansi.Reset())
		for _, n := range res.Nodes {
			flag := ""
			if !n.Active {
				flag = ansi.Color(ansi.Red) + "*" + ansi.Reset()
			} else {
				flag = " "
			}
			s.writeln(fmt.Sprintf("%s%-18s %-25s %-20s %s",
				flag, n.Addr4D(), n.Sysop, n.Location, n.Name))
		}
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightYellow) +
			"  [N]ext  [P]rev  [S]earch  [Network="+network+"]  [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("Nodelist: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "N":
			if page < res.Pages {
				page++
			}
		case "P":
			if page > 1 {
				page--
			}
		case "S":
			s.write(ansi.Prompt("Search (sysop/name/location/address): "))
			query = strings.TrimSpace(s.readline())
			page = 1
		case "NET", "NETWORK":
			s.write(ansi.Prompt("Network name: "))
			network = strings.TrimSpace(s.readline())
			if network == "" {
				network = "FidoNet"
			}
			page = 1
			query = ""
		case "Q", "":
			return
		}
		_ = cfg
	}
}

// echoFlagConference lets the sysop toggle echo/local on a conference and set the AREA tag.
func (s *session) echoFlagConference() {
	confs, err := s.deps.Conferences.List()
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
		return
	}
	s.writeln(ansi.Header("Conference Echo Settings"))
	for _, c := range confs {
		echo := ansi.Color(ansi.BrightCyan) + "LOCAL" + ansi.Reset()
		tag := ""
		if c.Echo {
			echo = ansi.Color(ansi.BrightGreen) + "ECHO " + ansi.Reset()
			tag = " tag=" + c.EchoTag
			if c.UplinkAddr != "" {
				tag += " uplink=" + c.UplinkAddr
			}
		}
		s.writeln(fmt.Sprintf("  %3d  %s  %-25s %s%s", c.ID, echo, c.Name, c.Network, tag))
	}
	s.writeln("")
	s.write(ansi.Prompt("Conference ID to edit (Enter=cancel): "))
	idStr := strings.TrimSpace(s.readline())
	if idStr == "" {
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return
	}
	conf, err := s.deps.Conferences.Get(id)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Conference not found."))
		return
	}
	s.writeln(fmt.Sprintf("  Current: echo=%v  tag=%q  uplink=%q  network=%q",
		conf.Echo, conf.EchoTag, conf.UplinkAddr, conf.Network))
	s.write(ansi.Prompt("Echo area? [y/N]: "))
	conf.Echo = strings.ToUpper(strings.TrimSpace(s.readline())) == "Y"
	if conf.Echo {
		s.write(ansi.Prompt("AREA tag (e.g. FIDO_GENERAL): "))
		conf.EchoTag = strings.TrimSpace(s.readline())
		s.write(ansi.Prompt("From name policy [R]eal / [A]lias / [N]anonymous [R]: "))
		switch strings.ToUpper(strings.TrimSpace(s.readline())) {
		case "A":
			conf.EchoFromName = conferences.EchoFromAlias
		case "N":
			conf.EchoFromName = conferences.EchoFromAnonymous
		default:
			conf.EchoFromName = conferences.EchoFromReal
		}
		s.write(ansi.Prompt("Override uplink address (blank=default): "))
		conf.UplinkAddr = strings.TrimSpace(s.readline())
		s.write(ansi.Prompt("Network name (blank=primary): "))
		conf.Network = strings.TrimSpace(s.readline())
	} else {
		conf.EchoTag = ""
		conf.UplinkAddr = ""
	}
	if err := s.deps.Conferences.Update(conf); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Save error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Saved."))
}

// pickFidoNetwork prompts the sysop to choose which configured FidoNet
// network to act on, when more than one is enabled. Returns nil (after
// printing a message) if FidoNet isn't enabled or no network is found.
// verb is used in the prompt, e.g. "poll", "ping", "trace".
func (s *session) pickFidoNetwork(verb string) *fido.NetworkDef {
	cfg := config.Get()
	nets := cfg.Fido.AllNetworks()
	if len(nets) == 0 || !nets[0].Enabled {
		s.writeln(ansi.Colorize(ansi.Yellow, "FidoNet not enabled."))
		return nil
	}

	netName := nets[0].Name
	if len(nets) > 1 {
		for i, n := range nets {
			s.writeln(fmt.Sprintf("  %d. %s (%s → %s)", i+1, n.Name, n.Address, n.Uplink))
		}
		s.write(ansi.Prompt(fmt.Sprintf("Network # to %s (Enter=1): ", verb)))
		numStr := strings.TrimSpace(s.readline())
		if num, err := strconv.Atoi(numStr); err == nil && num >= 1 && num <= len(nets) {
			netName = nets[num-1].Name
		}
	}

	target := cfg.Fido.NetworkByName(netName)
	if target == nil {
		s.writeln(ansi.Colorize(ansi.Red, "Network not found."))
		return nil
	}
	return target
}

// fidoPoll calls the uplink via BinkP, sending any outbound bundles.
// fidoLoadNodelist fetches and imports a fresh nodelist for a chosen
// network right now, instead of waiting for the scheduler's next tick —
// see internal/fido/nodelistfetch.go.
func (s *session) fidoLoadNodelist() {
	target := s.pickFidoNetwork("fetch a nodelist for")
	if target == nil {
		return
	}
	s.writeln(ansi.Colorize(ansi.White, "Fetching nodelist from "+target.EffectiveNodelistURL()+"…"))
	if !target.NodelistFetchEnabled() {
		s.writeln(ansi.Colorize(ansi.Yellow, "No nodelist_url configured for "+target.Name+" — set one in network config or use Import File."))
		return
	}
	result, err := fido.FetchAndImport(target, s.deps.Messages.DB())
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Nodelist fetch error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"Nodelist import complete: %d inserted, %d updated, %d skipped.",
		result.Inserted, result.Updated, result.Skipped)))
	for _, e := range result.Errors {
		s.writeln(ansi.Colorize(ansi.Red, "  Error: "+e))
	}
}

func (s *session) fidoPoll() {
	target := s.pickFidoNetwork("poll")
	if target == nil {
		return
	}

	s.writeln(ansi.Colorize(ansi.White, fmt.Sprintf("Polling %s uplink %s…", target.Name, target.Uplink)))

	result := fido.PollAndToss(target, s.deps.Messages, s.deps.Conferences, config.Get().Sysop.Name, s.deps.Files, config.Get().Paths.Files)
	if result.Poll.Error != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Poll error: "+result.Poll.Error.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"Poll complete: sent %d, received %d file(s).",
		len(result.Poll.Sent), len(result.Poll.Received))))

	if result.Toss != nil {
		s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
			"Auto-toss complete: %d packet(s), %d imported, %d skipped, %d held.",
			result.Toss.Packets, result.Toss.Imported, result.Toss.Skipped, result.Toss.Orphaned)))
		for _, e := range result.Toss.Errors {
			s.writeln(ansi.Colorize(ansi.Red, "  Toss error: "+e))
		}
	}
}

// netmailMenu lists, reads, or sends FidoNet netmail.
func (s *session) netmailMenu() {
	for {
		count, _ := s.deps.Messages.CountNetmail(s.user.Name, s.user.Sysop)
		s.writeln(ansi.Header("FidoNet NetMail"))
		if count > 0 {
			s.writeln(ansi.Color(ansi.BrightWhite) +
				fmt.Sprintf("  %d message(s) waiting", count) + ansi.Reset())
		}
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [R]ead   [S]end   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("NetMail command: "))
		switch strings.ToUpper(strings.TrimSpace(s.readline())) {
		case "R":
			s.write(ansi.Prompt("Start at message # (Enter=oldest): "))
			startStr := strings.TrimSpace(s.readline())
			start := 1
			if n, err := strconv.Atoi(startStr); err == nil {
				start = n
			}
			s.netmailRead(start)
		case "S":
			s.netmailCompose()
		case "Q", "":
			return
		}
	}
}

func (s *session) netmailRead(startNum int) {
	msgs, err := s.deps.Messages.ListNetmail(s.user.Name, s.user.Sysop, startNum, 20)
	if err != nil || len(msgs) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No netmail."))
		return
	}
	for _, m := range msgs {
		s.displayMessageHeader(m)
		if m.FidoOrigin != "" {
			s.writeln(ansi.Color(ansi.White) + "  Addr: " + m.FidoOrigin + ansi.Reset())
		}
		s.writeln(m.Body)

		s.statMsgsRead++
		s.write(ansi.Prompt("[N]ext / [R]eply / [T]hread / [Q]uit: "))
		switch strings.ToUpper(strings.TrimSpace(s.readline())) {
		case "Q":
			return
		case "R":
			s.netmailReply(m)
		case "T":
			s.showThread(m)
		}
	}
	s.writeln(ansi.Colorize(ansi.Yellow, "End of netmail."))
}

// netmailReply composes an outbound FidoNet netmail in reply to a received message.
func (s *session) netmailReply(orig *messages.Message) {
	cfg := config.Get()
	if orig.FidoOrigin == "" {
		s.writeln(ansi.Colorize(ansi.Yellow, "Cannot reply — no FidoNet origin address."))
		return
	}

	subj := orig.Subject
	if !strings.HasPrefix(strings.ToUpper(subj), "RE:") {
		subj = "RE: " + subj
	}

	quoted := ""
	for _, line := range strings.Split(strings.ReplaceAll(orig.Body, "\r\n", "\n"), "\n") {
		quoted += "> " + line + "\r\n"
	}
	quoted += "\r\n"

	result := s.runEditor(subj, quoted)
	if result.Aborted || result.Body == "" {
		s.writeln(ansi.Colorize(ansi.Yellow, "Reply aborted."))
		return
	}

	msg := &fido.NetmailMsg{
		FromName:   s.user.Name,
		FromAddr:   cfg.Fido.Address,
		ToName:     orig.FromName,
		ToAddr:     orig.FidoOrigin,
		Subject:    subj,
		Body:       result.Body,
		ReplyMsgID: orig.FidoMsgID,
		AuthorLang: fido.NormalizeLangCode(s.user.Locale),
	}

	nd := cfg.Fido.AllNetworks()[0]
	nextHop, err := fido.RouteAddr(s.deps.Messages.DB(), msg, &nd)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Routing error: "+err.Error()))
		return
	}
	outDir := fido.OutboundDir(nd.OutboundDir, nextHop, nd.UplinkAddr(), false)
	origAddr, _ := fido.ParseAddr(cfg.Fido.Address)

	pktPath, err := fido.WritePKT(origAddr, nextHop, nd.Password, outDir, []*fido.NetmailMsg{msg}, nd.Name)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error writing PKT: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"NetMail reply queued → %s (next hop: %s)", pktPath, nextHop.String())))
}

// netmailCompose lets the user write a FidoNet netmail.
func (s *session) netmailCompose() {
	cfg := config.Get()
	ndb := fido.OpenNodelistDB(s.deps.Messages.DB())

	s.writeln(ansi.Header("Send FidoNet NetMail"))
	s.write(ansi.Prompt("To (FidoNet address, e.g. 1:234/567): "))
	toAddr := strings.TrimSpace(s.readline())
	if toAddr == "" {
		return
	}

	// Validate address against nodelist.
	addr, err := fido.ParseAddr(toAddr)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid address: "+err.Error()))
		return
	}

	// For point addresses, look up the boss node.
	lookupAddr := addr
	if addr.Point != 0 {
		lookupAddr = fido.Addr{Zone: addr.Zone, Net: addr.Net, Node: addr.Node}
		s.writeln(ansi.Color(ansi.Yellow) + fmt.Sprintf("  Point address — routing via boss %s", lookupAddr.String()) + ansi.Reset())
	}

	node, err := ndb.LookupAddr("", lookupAddr)
	if err != nil || node == nil {
		s.writeln(ansi.Colorize(ansi.Yellow, fmt.Sprintf("Warning: address %s not found in nodelist.", lookupAddr.String())))
		s.write(ansi.Prompt("Send anyway? [y/N]: "))
		if strings.ToUpper(strings.TrimSpace(s.readline())) != "Y" {
			return
		}
	} else {
		s.writeln(ansi.Color(ansi.BrightGreen) +
			fmt.Sprintf("  Found: %s @ %s (%s, %s)", node.Sysop, node.Name, node.Location, node.Phone) +
			ansi.Reset())
	}

	s.write(ansi.Prompt("To name: "))
	toName := strings.TrimSpace(s.readline())
	if toName == "" {
		if node != nil {
			toName = node.Sysop
		} else {
			toName = "Sysop"
		}
	}

	s.write(ansi.Prompt("Subject: "))
	subject := strings.TrimSpace(s.readline())

	s.writeln(ansi.Color(ansi.White) + "Enter message body (blank line to end):" + ansi.Reset())
	body := s.readBody()

	s.write(ansi.Prompt("Crash mail (send direct)? [y/N]: "))
	crash := strings.ToUpper(strings.TrimSpace(s.readline())) == "Y"

	msg := &fido.NetmailMsg{
		FromName:   s.user.Name,
		FromAddr:   cfg.Fido.Address,
		ToName:     toName,
		ToAddr:     toAddr,
		Subject:    subject,
		Body:       body,
		Crash:      crash,
		Network:    "",
		AuthorLang: fido.NormalizeLangCode(s.user.Locale),
	}

	// Determine routing and write PKT.
	nd := cfg.Fido.AllNetworks()[0]
	nextHop, err := fido.RouteAddr(s.deps.Messages.DB(), msg, &nd)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Routing error: "+err.Error()))
		return
	}
	outDir := fido.OutboundDir(nd.OutboundDir, nextHop, nd.UplinkAddr(), crash)
	origAddr, _ := fido.ParseAddr(cfg.Fido.Address)

	pktPath, err := fido.WritePKT(origAddr, nextHop, nd.Password, outDir, []*fido.NetmailMsg{msg}, nd.Name)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error writing PKT: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"NetMail queued → %s (next hop: %s)", pktPath, nextHop.String())))
}

func (s *session) sysopScanFiles() {
	s.writeln(ansi.Header("Scan File Directories"))
	s.writeln(ansi.Color(ansi.BrightWhite) +
		"  Scanning disk folders and updating the file catalog..." + ansi.Reset())

	totals, err := s.deps.Files.ScanAll(s.user.Name)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Scan failed: "+err.Error()))
		return
	}
	if totals.Dirs == 0 && totals.NewAreas == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No file directories configured."))
		return
	}

	if totals.NewAreas > 0 {
		s.writeln(ansi.Colorize(ansi.BrightGreen,
			fmt.Sprintf("  Registered %d new file area%s: %s",
				totals.NewAreas, pluralS(totals.NewAreas), strings.Join(totals.NewAreaNames, ", "))))
	}

	for _, res := range totals.Results {
		for _, added := range res.AddedFiles {
			s.recordFileUpload(added.Size)
		}
		restored := ""
		if res.Restored > 0 {
			restored = fmt.Sprintf("  restored: %d", res.Restored)
		}
		s.writeln(fmt.Sprintf("  %s%-20s%s  on disk: %d  added: %d  missing: %d%s",
			ansi.Color(ansi.BrightCyan), res.DirName, ansi.Reset(),
			res.OnDisk, res.Added, res.Missing, restored))
	}
	s.rebuildLocalFile()
	s.writeln("")
	s.writeln(ansi.Colorize(ansi.BrightGreen,
		fmt.Sprintf("Scan complete — %d director%s, %d file area(s) added, %d file(s) added, %d marked missing.",
			totals.Dirs, pluralS(totals.Dirs), totals.NewAreas, totals.Added, totals.Missing)))
}

func (s *session) sysopEditFileDesc() {
	s.listDirs()
	s.write(ansi.Prompt("Directory # : "))
	dirInput := strings.TrimSpace(s.readline())
	dirID, err := strconv.ParseInt(dirInput, 10, 64)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid directory number."))
		return
	}
	dir, err := s.deps.Files.GetDir(dirID)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Directory not found."))
		return
	}
	fileList, err := s.deps.Files.ListFiles(dirID)
	if err != nil || len(fileList) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No files in this directory."))
		return
	}
	s.writeln(ansi.Header("Directory: " + dir.Name))
	for _, f := range fileList {
		s.writeln(fmt.Sprintf("  %s%-20s%s  %s",
			ansi.Color(ansi.BrightWhite), f.Filename, ansi.Reset(), f.Description))
	}
	s.write(ansi.Prompt("Filename to edit: "))
	filename := strings.TrimSpace(s.readline())
	if filename == "" {
		return
	}
	var found bool
	for _, f := range fileList {
		if strings.EqualFold(f.Filename, filename) {
			filename = f.Filename
			found = true
			break
		}
	}
	if !found {
		s.writeln(ansi.Colorize(ansi.Red, "File not found."))
		return
	}
	s.write(ansi.Prompt("New description: "))
	desc := strings.TrimSpace(s.readline())
	if err := s.deps.Files.UpdateDescription(dirID, filename, desc); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Description updated."))
	s.rebuildLocalFile()
}

func (s *session) recordFileUpload(size int64) {
	s.statFilesUp++
	if s.user == nil {
		return
	}
	s.user.Uploads++
	s.user.BytesUploaded += size
	_ = s.deps.Users.Update(s.user)
}

func (s *session) rebuildLocalFile() {
	cfg := config.Get()
	if err := s.deps.Files.BuildLocalFile(cfg.BBS.Name); err != nil {
		s.writeln(ansi.Colorize(ansi.Yellow, "Note: could not rebuild file listing: "+err.Error()))
	}
}

func pluralS(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func (s *session) sysopCreateConference() {
	s.write(ansi.Prompt("Conference name: "))
	name := strings.TrimSpace(s.readline())
	if name == "" {
		return
	}
	s.write(ansi.Prompt("Description: "))
	desc := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("Public? [Y/n]: "))
	pub := strings.ToUpper(strings.TrimSpace(s.readline())) != "N"

	c := &conferences.Conference{
		Name:        name,
		Description: desc,
		Public:      pub,
		ReadSec:     10,
		WriteSec:    10,
		SysopSec:    110,
	}
	if err := s.deps.Conferences.Create(c); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Conference '%s' created with ID %d.", name, c.ID)))
}

// ── Display files ─────────────────────────────────────────────────────────────

// displayVars builds a *display.Vars for the current session.
func (s *session) displayVars() *display.Vars {
	cfg := config.Get()
	v := &display.Vars{
		BBSName:   cfg.BBS.Name,
		SysopName: cfg.Sysop.Name,
		Node:      s.nodeID,
		TimeLeft:  s.timeLeft(),
		TimeOn:    int(time.Since(s.startTime).Minutes()),
	}
	if s.user != nil {
		v.Name = s.user.Name
		v.City = s.user.City
		v.Security = s.user.SecurityLevel
		v.NumCalls = s.user.TimesOnline
	}
	return v
}

// timeLeft returns minutes remaining in this call (0 if unlimited).
func (s *session) timeLeft() int {
	cfg := config.Get()
	limit := cfg.Session.TimePerCallMins
	if limit <= 0 {
		return 0
	}
	elapsed := int(time.Since(s.startTime).Minutes())
	left := limit - elapsed
	if left < 0 {
		return 0
	}
	return left
}

// showDisplayFile renders and sends a named display file if it exists.
// Silently does nothing if the file is not found.
func (s *session) showDisplayFile(name string) {
	cfg := config.Get()
	text, err := display.Render(cfg.Session.DisplayDir, name, s.displayVars())
	if err != nil {
		return // file not found — silently skip
	}
	s.write(text)
}

// showNewMessages prints the count of new messages per conference since last visit.
func (s *session) showNewMessages() {
	if s.user == nil {
		return
	}
	counts, err := s.deps.Users.NewMessageCounts(s.user.ID)
	if err != nil || len(counts) == 0 {
		return
	}
	total := 0
	for _, n := range counts {
		total += n
	}
	if total == 0 {
		return
	}
	s.writeln(ansi.Color(ansi.BrightYellow) +
		fmt.Sprintf("  *** You have %d new message(s) waiting ***", total) +
		ansi.Reset())
	// Per-conference breakdown.
	for confID, n := range counts {
		name := fmt.Sprintf("Conference %d", confID)
		if conf, err := s.deps.Conferences.Get(confID); err == nil {
			name = conf.Name
		}
		s.writeln(fmt.Sprintf("      %-30s  %d new", name, n))
	}
	s.writeln("")
}

// ── User Profile ──────────────────────────────────────────────────────────────

func (s *session) profileMenu() {
	for {
		s.writeln(ansi.Header("User Profile"))
		s.writeln(fmt.Sprintf("  %sName%s       : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.Name))
		s.writeln(fmt.Sprintf("  %sReal name%s  : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.RealName))
		s.writeln(fmt.Sprintf("  %sCity%s       : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.City))
		s.writeln(fmt.Sprintf("  %sANSI%s       : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), yesNo(s.user.ANSI)))
		s.writeln(fmt.Sprintf("  %sEditor%s     : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), editorLabel(s.user.EditorType)))
		s.writeln(fmt.Sprintf("  %sPage length%s: %d", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.PageLength))
		s.writeln(fmt.Sprintf("  %sProtocol%s   : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.XferProtocol))
		s.writeln(fmt.Sprintf("  %sExpert mode%s: %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), yesNo(s.user.ExpertMode)))
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [C]ity   [N]ame   [P]assword   [A]NSI   [M]sg editor   [L]ines/page   [X]fer   [E]xpert   [T]okens   [J]oin network   [Q]uit" +
			ansi.Reset())
		s.write(ansi.Prompt("Profile: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "C":
			s.write(ansi.Prompt("New city/state: "))
			city := strings.TrimSpace(s.readline())
			if city != "" {
				s.user.City = city
				_ = s.deps.Users.Update(s.user)
				s.writeln(ansi.Colorize(ansi.BrightGreen, "City updated."))
			}
		case "N":
			s.write(ansi.Prompt("Real name (FidoNet): "))
			realName := strings.TrimSpace(s.readline())
			if realName != "" {
				s.user.RealName = realName
				_ = s.deps.Users.Update(s.user)
				s.writeln(ansi.Colorize(ansi.BrightGreen, "Real name updated."))
			}
		case "P":
			s.write(ansi.Prompt("Current password: "))
			old := s.readlineHidden()
			if _, err := s.deps.Users.Authenticate(s.user.Name, old); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "\r\nIncorrect password."))
				continue
			}
			s.write(ansi.Prompt("\r\nNew password: "))
			p1 := s.readlineHidden()
			s.write(ansi.Prompt("\r\nConfirm: "))
			p2 := s.readlineHidden()
			if p1 == "" || p1 != p2 {
				s.writeln(ansi.Colorize(ansi.Red, "\r\nPasswords do not match."))
				continue
			}
			if err := s.deps.Users.SetPassword(s.user.ID, p1); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "\r\nError: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, "\r\nPassword changed."))
		case "A":
			s.user.ANSI = !s.user.ANSI
			_ = s.deps.Users.Update(s.user)
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("ANSI is now %s.", yesNo(s.user.ANSI))))
		case "M":
			s.selectEditor()
		case "L":
			s.write(ansi.Prompt("Lines per page (5-99): "))
			n, _ := strconv.Atoi(strings.TrimSpace(s.readline()))
			if n >= 5 && n <= 99 {
				s.user.PageLength = n
				_ = s.deps.Users.Update(s.user)
				s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Page length set to %d.", n)))
			}
		case "X":
			s.writeln("  Protocols: [Z]modem  [X]modem  [Y]modem  [N]one")
			s.write(ansi.Prompt("Protocol: "))
			p := strings.ToUpper(strings.TrimSpace(s.readline()))
			if p == "Z" || p == "X" || p == "Y" || p == "N" {
				s.user.XferProtocol = p
				_ = s.deps.Users.Update(s.user)
				s.writeln(ansi.Colorize(ansi.BrightGreen, "Protocol updated."))
			}
		case "E":
			s.user.ExpertMode = !s.user.ExpertMode
			_ = s.deps.Users.Update(s.user)
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Expert mode is now %s.", yesNo(s.user.ExpertMode))))
		case "T":
			s.manageAPITokens()
		case "J":
			s.applyToJoinNetwork()
		case "Q", "":
			return
		}
	}
}

// applyToJoinNetwork lets any logged-in caller apply to join a FidoNet-
// compatible network this BBS itself hosts (NetworkDef.IsHub() — a blank
// Uplink, e.g. VirtNet). Submits a fido_join_requests row for the sysop to
// review/approve via fidoJoinRequestsMenu — see internal/fido/members.go.
func (s *session) applyToJoinNetwork() {
	cfg := config.Get()
	var hubs []fido.NetworkDef
	for _, nd := range cfg.Fido.AllNetworks() {
		if nd.Enabled && nd.IsHub() {
			hubs = append(hubs, nd)
		}
	}
	if len(hubs) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "This BBS doesn't host any network you can apply to join."))
		return
	}

	target := hubs[0]
	if len(hubs) > 1 {
		s.writeln(ansi.Header("Apply to Join a Network"))
		for i, n := range hubs {
			s.writeln(fmt.Sprintf("  %d. %s (%s)", i+1, n.Name, n.Address))
		}
		s.write(ansi.Prompt("Network # (Enter=1): "))
		if num, err := strconv.Atoi(strings.TrimSpace(s.readline())); err == nil && num >= 1 && num <= len(hubs) {
			target = hubs[num-1]
		}
	}

	s.writeln(ansi.Header("Apply to Join " + target.Name))
	s.write(ansi.Prompt("Your BBS/system name: "))
	bbsName := strings.TrimSpace(s.readline())
	if bbsName == "" {
		s.writeln(ansi.Colorize(ansi.Yellow, "Cancelled."))
		return
	}
	s.write(ansi.Prompt("Sysop name (Enter=your username): "))
	sysopName := strings.TrimSpace(s.readline())
	if sysopName == "" {
		sysopName = s.user.Name
	}
	s.write(ansi.Prompt("Location (city, state/country): "))
	location := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("Contact (email or phone): "))
	contact := strings.TrimSpace(s.readline())
	s.write(ansi.Prompt("Preferred net number (Enter=sysop's choice): "))
	var netPtr *int
	if numStr := strings.TrimSpace(s.readline()); numStr != "" {
		if num, err := strconv.Atoi(numStr); err == nil {
			netPtr = &num
		}
	}
	s.write(ansi.Prompt("BinkP host:port, if you already run mailer software (Enter=skip): "))
	binkpHost := strings.TrimSpace(s.readline())

	mdb := fido.OpenMembersDB(s.deps.Messages.DB())
	id, err := mdb.SubmitJoinRequest(&fido.JoinRequest{
		Network:           target.Name,
		RequestedByUserID: int(s.user.ID),
		BBSName:           bbsName,
		SysopName:         sysopName,
		Location:          location,
		Contact:           contact,
		RequestedNet:      netPtr,
		BinkpHost:         binkpHost,
	})
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error submitting application: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"Application #%d submitted. The sysop will review it and assign your node address.", id)))
}

// manageAPITokens lets a user generate or revoke their own API tokens, used
// by the VirtAnd (Android) client app to authenticate against internal/userapi
// without sending their BBS password.
func (s *session) manageAPITokens() {
	for {
		s.writeln("")
		s.writeln(ansi.Header("API Tokens (VirtAnd)"))
		tokens, err := s.deps.Users.ListAPITokens(s.user.ID)
		if err != nil {
			s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
			return
		}
		if len(tokens) == 0 {
			s.writeln("  (no tokens issued yet)")
		}
		for i, t := range tokens {
			status := ansi.Colorize(ansi.BrightGreen, "active")
			if t.RevokedAt != "" {
				status = ansi.Colorize(ansi.Red, "revoked")
			}
			label := t.DeviceLabel
			if label == "" {
				label = "(unlabeled)"
			}
			s.writeln(fmt.Sprintf("  %2d) %-20s created %s  [%s]", i+1, label, t.CreatedAt, status))
		}
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [G]enerate new   [R]evoke   [Q]uit" +
			ansi.Reset())
		s.write(ansi.Prompt("Tokens: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "G":
			s.write(ansi.Prompt("Device label (e.g. \"My Phone\"): "))
			label := strings.TrimSpace(s.readline())
			raw, err := s.deps.Users.CreateAPIToken(s.user.ID, label)
			if err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			s.writeln("")
			s.writeln(ansi.Colorize(ansi.BrightYellow, "Your new token (shown once — copy it now):"))
			s.writeln("  " + raw)
			s.writeln("")
		case "R":
			s.write(ansi.Prompt("Token # to revoke: "))
			n, _ := strconv.Atoi(strings.TrimSpace(s.readline()))
			if n < 1 || n > len(tokens) {
				s.writeln(ansi.Colorize(ansi.Red, "Invalid selection."))
				continue
			}
			if err := s.deps.Users.RevokeAPIToken(s.user.ID, tokens[n-1].ID); err != nil {
				s.writeln(ansi.Colorize(ansi.Red, "Error: "+err.Error()))
				continue
			}
			s.writeln(ansi.Colorize(ansi.BrightGreen, "Token revoked."))
		case "Q", "":
			return
		}
	}
}

// selectEditor shows the editor selection menu and saves the user's preference.
func (s *session) selectEditor() {
	s.writeln("")
	s.writeln(ansi.Header("Message Editor Selection"))
	s.writeln("")
	s.writeln(fmt.Sprintf("  %s1%s  Simple line editor    — classic PCBoard style, no ANSI required",
		ansi.Color(ansi.BrightYellow), ansi.Reset()))
	if s.user.ANSI {
		s.writeln(fmt.Sprintf("  %s2%s  Full-screen editor    — VT100/ANSI required, word-wrap, cut/paste, arrow keys",
			ansi.Color(ansi.BrightYellow), ansi.Reset()))
	} else {
		s.writeln(fmt.Sprintf("  %s2%s  Full-screen editor    — %s(requires ANSI — enable ANSI first)%s",
			ansi.Color(ansi.BrightYellow), ansi.Reset(), ansi.Color(ansi.Red), ansi.Reset()))
	}
	s.writeln("")
	s.write(ansi.Prompt("Select editor [1/2] (Enter=cancel): "))
	choice := strings.TrimSpace(s.readline())
	switch choice {
	case "1":
		s.user.EditorType = editor.EditorSimple
		_ = s.deps.Users.Update(s.user)
		s.writeln(ansi.Colorize(ansi.BrightGreen, "Editor set to: Simple line editor."))
	case "2":
		if !s.user.ANSI {
			s.writeln(ansi.Colorize(ansi.Red, "Enable ANSI first (option [A] in profile)."))
			return
		}
		s.user.EditorType = editor.EditorFullScreen
		_ = s.deps.Users.Update(s.user)
		s.writeln(ansi.Colorize(ansi.BrightGreen, "Editor set to: Full-screen ANSI editor."))
	case "":
		// cancelled
	default:
		s.writeln(ansi.Colorize(ansi.Red, "Invalid selection."))
	}
}

// editorLabel returns a human-readable name for an editor type string.
func editorLabel(t string) string {
	switch t {
	case editor.EditorFullScreen:
		return "Full-screen ANSI"
	default:
		return "Simple (line-by-line)"
	}
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// ── Door Games ────────────────────────────────────────────────────────────────

func (s *session) doorMenu() {
	cfg := config.Get()
	if len(cfg.Doors) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No door games are configured."))
		return
	}
	for {
		s.writeln(ansi.Header("Door Games"))
		for i, d := range cfg.Doors {
			if s.user.SecurityLevel < d.MinSecurity {
				continue
			}
			s.writeln(fmt.Sprintf("  %s%2d%s  %-20s  %s",
				ansi.Color(ansi.BrightCyan), i+1, ansi.Reset(),
				d.Name, d.Description))
		}
		s.write(ansi.Prompt("\r\nDoor # (Enter=quit): "))
		input := strings.TrimSpace(s.readline())
		if input == "" {
			return
		}
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 || n > len(cfg.Doors) {
			s.writeln(ansi.Colorize(ansi.Red, "Invalid selection."))
			continue
		}
		chosen := cfg.Doors[n-1]
		if s.user.SecurityLevel < chosen.MinSecurity {
			s.writeln(ansi.Colorize(ansi.Red, "Access denied."))
			continue
		}
		s.runDoor(chosen)
		return
	}
}

func (s *session) runDoor(cfg door.Config) {
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusDoor, "Door: "+cfg.Name, s.user.ID, s.user.Name, s.user.City)

	sess := door.Session{
		NodeID:        s.nodeID,
		UserName:      s.user.Name,
		City:          s.user.City,
		SecurityLevel: s.user.SecurityLevel,
		TimesOnline:   s.user.TimesOnline,
		TimeLeftMins:  s.timeLeft(),
		ANSI:          s.user.ANSI,
		BaudRate:      38400,
		BBSName:       config.Get().BBS.Name,
		SysopName:     config.Get().Sysop.Name,
	}

	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Launching %s... (type CTRL+] to abort)", cfg.Name)))
	if err := door.Run(s.rw, cfg, sess); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Door error: "+err.Error()))
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Returned from "+cfg.Name+"."))
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusMain, "Main Menu", s.user.ID, s.user.Name, s.user.City)
}

// ── PPL / PPE ─────────────────────────────────────────────────────────────────

func (s *session) ppeMenu() {
	s.writeln(ansi.Header("Run PPE Program"))
	s.write(ansi.Prompt("PPE file path (.PPS): "))
	path := strings.TrimSpace(s.readline())
	if path == "" {
		return
	}
	s.runPPE(path)
}

// pplReadLine handles INPUTSTR/INPUTINT prompts for PPE programs.
func (s *session) pplReadLine(prompt string) string {
	if prompt != "" {
		s.write(prompt)
	}
	return s.readline()
}

// bbsStats holds the counters shown by the main-menu Stats screen and GETSTATS.
type bbsStats struct {
	UserUploads         int
	UserDownloads       int
	UserBytesUploaded   int64
	UserBytesDownloaded int64
	UserLastLoginDate   string
	UserLastLoginTime   string
	UserTimesOn         int
	NewMsgsTotal        int
	SessMsgsRead        int
	SessMsgsLeft        int
	SessFilesDown       int
	SessFilesUp         int
	SessMinutes         int
	SessTimeLeft        int
	BBSCallsToday       int
	BBSUniqueToday      int
	BBSMsgTotal         int
	BBSConfCount        int
	BBSFileTotal        int
	BBSFileToday        int
	BBSFileMonth        int
}

func (s *session) gatherStats() bbsStats {
	var st bbsStats
	if s.user == nil {
		return st
	}
	st.UserUploads = s.user.Uploads
	st.UserDownloads = s.user.Downloads
	st.UserBytesUploaded = s.user.BytesUploaded
	st.UserBytesDownloaded = s.user.BytesDownloaded
	st.UserLastLoginDate = s.user.LastLoginDate
	st.UserLastLoginTime = s.user.LastLoginTime
	st.UserTimesOn = s.user.TimesOnline
	st.SessMsgsRead = s.statMsgsRead
	st.SessMsgsLeft = s.statMsgsLeft
	st.SessFilesDown = s.statFilesDown
	st.SessFilesUp = s.statFilesUp
	st.SessMinutes = int(time.Since(s.startTime).Minutes())
	st.SessTimeLeft = s.timeLeft()

	if counts, err := s.deps.Users.NewMessageCounts(s.user.ID); err == nil {
		for _, n := range counts {
			st.NewMsgsTotal += n
		}
	}
	if unique, total, err := s.deps.Callers.DailyStats(); err == nil {
		st.BBSUniqueToday = unique
		st.BBSCallsToday = total
	}
	if n, err := s.deps.Messages.TotalCount(); err == nil {
		st.BBSMsgTotal = n
	}
	if confs, err := s.deps.Conferences.List(); err == nil {
		st.BBSConfCount = len(confs)
	}
	if cat, err := s.deps.Files.GetCatalogStats(); err == nil {
		st.BBSFileTotal = cat.Total
		st.BBSFileToday = cat.Today
		st.BBSFileMonth = cat.LastMonth
	}
	return st
}

const statsPageLines = 23

// statsPager writes the stats screen and pauses every 23 lines (plus one
// explicit pause right before the first section header, so the header/
// banner block isn't split mid-way through by wherever the 23-line count
// happens to land).
type statsPager struct {
	s     *session
	lines int
}

func (p *statsPager) writeln(text string) {
	p.s.writeln(text)
	p.lines++
	if p.lines >= statsPageLines {
		p.pause()
	}
}

func (p *statsPager) pause() {
	p.s.write(ansi.Prompt("\r\n  Press a key to continue... "))
	_ = p.s.readline()
	p.s.writeln("")
	p.lines = 0
}

func (p *statsPager) section(title string) {
	p.writeln("")
	p.writeln(ansi.Bold() + ansi.Color(ansi.BrightYellow) + "  ► " + title + ansi.Reset())
	p.writeln(ansi.Color(ansi.BrightCyan) + "  " + strings.Repeat("─", 58) + ansi.Reset())
}

func (p *statsPager) line(label, value string) {
	p.writeln(fmt.Sprintf("  %s%-14s%s %s%s",
		ansi.Color(ansi.BrightCyan), label, ansi.Reset(),
		ansi.Color(ansi.White), value+ansi.Reset()))
}

func (p *statsPager) lineHighlight(label, value string) {
	p.writeln(fmt.Sprintf("  %s%-14s%s %s%s",
		ansi.Color(ansi.BrightCyan), label, ansi.Reset(),
		ansi.Color(ansi.BrightYellow), value+ansi.Reset()))
}

// showStats displays BBS/user statistics (same data as ppe/stats.pps) using ANSI.
func (s *session) showStats() {
	cfg := config.Get()
	st := s.gatherStats()
	p := &statsPager{s: s}

	s.write(ansi.ClearScreen())
	const statsInnerW = 58
	statsBorder := ansi.Bold() + ansi.Color(ansi.BrightCyan)
	statsLine := func(inner string) string {
		return statsBorder + "  ║" + ansi.Reset() + inner + statsBorder + "║" + ansi.Reset()
	}

	p.writeln("")
	p.writeln(statsBorder + "  ╔" + strings.Repeat("═", statsInnerW) + "╗" + ansi.Reset())
	p.writeln(statsLine(ansi.Bold() + ansi.Color(ansi.BrightWhite) + padCenter("BBS Statistics", statsInnerW)))
	p.writeln(statsBorder + "  ╠" + strings.Repeat("═", statsInnerW) + "╣" + ansi.Reset())
	p.writeln(statsLine("  " + ansi.Bold() + ansi.Color(ansi.BrightYellow) + padRight(cfg.BBS.Name, statsInnerW-2)))
	p.writeln(statsBorder + "  ╚" + strings.Repeat("═", statsInnerW) + "╝" + ansi.Reset())
	p.writeln("")
	p.pause() // pause right before the first section, not wherever the 23-line counter happens to land

	p.section("This Call — " + s.user.Name)
	p.line("Node", fmt.Sprintf("%d", s.nodeID))
	p.line("Time on", fmt.Sprintf("%d min", st.SessMinutes))
	if st.SessTimeLeft > 0 {
		p.line("Time left", fmt.Sprintf("%d min", st.SessTimeLeft))
	}
	p.line("Msgs read", fmt.Sprintf("%d", st.SessMsgsRead))
	p.line("Msgs posted", fmt.Sprintf("%d", st.SessMsgsLeft))
	p.line("Files down", fmt.Sprintf("%d", st.SessFilesDown))
	p.line("Files up", fmt.Sprintf("%d", st.SessFilesUp))

	p.section("Your Account")
	p.line("Times on", fmt.Sprintf("%d", st.UserTimesOn))
	p.line("Last login", st.UserLastLoginDate+" "+st.UserLastLoginTime)
	p.line("Uploads", fmt.Sprintf("%d  (%d KB)", st.UserUploads, st.UserBytesUploaded/1024))
	p.line("Downloads", fmt.Sprintf("%d  (%d KB)", st.UserDownloads, st.UserBytesDownloaded/1024))
	if st.NewMsgsTotal > 0 {
		p.lineHighlight("New mail", fmt.Sprintf("%d waiting", st.NewMsgsTotal))
	} else {
		p.line("New mail", "0")
	}

	p.section("System Today")
	p.line("Calls today", fmt.Sprintf("%d", st.BBSCallsToday))
	p.line("Unique users", fmt.Sprintf("%d", st.BBSUniqueToday))

	p.section("Message Base")
	p.line("Conferences", fmt.Sprintf("%d", st.BBSConfCount))
	p.line("Total msgs", fmt.Sprintf("%d", st.BBSMsgTotal))

	p.section("File Areas")
	p.line("Total files", fmt.Sprintf("%d", st.BBSFileTotal))
	p.line("Added today", fmt.Sprintf("%d", st.BBSFileToday))
	p.line("This session", fmt.Sprintf("%d", st.SessFilesUp))
	p.line("Last 30 days", fmt.Sprintf("%d", st.BBSFileMonth))

	p.writeln("")
	p.writeln(ansi.Color(ansi.BrightBlack) + strings.Repeat("─", 62) + ansi.Reset())
	s.writeln("")
}

func padRight(s string, width int) string {
	if len(s) >= width {
		if width <= 3 {
			return s[:width]
		}
		return s[:width-3] + "..."
	}
	return s + strings.Repeat(" ", width-len(s))
}

func padCenter(s string, width int) string {
	if len(s) >= width {
		return padRight(s, width)
	}
	left := (width - len(s)) / 2
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", width-len(s)-left)
}

// populatePplStats fills extended statistics into a PPL environment for GETSTATS.
func (s *session) populatePplStats(env *ppl.Environment) {
	if env == nil {
		return
	}
	st := s.gatherStats()
	env.UserUploads = st.UserUploads
	env.UserDownloads = st.UserDownloads
	env.UserBytesUploaded = st.UserBytesUploaded
	env.UserBytesDownloaded = st.UserBytesDownloaded
	env.UserLastLoginDate = st.UserLastLoginDate
	env.UserLastLoginTime = st.UserLastLoginTime
	env.SessMsgsRead = st.SessMsgsRead
	env.SessMsgsLeft = st.SessMsgsLeft
	env.SessFilesDown = st.SessFilesDown
	env.SessFilesUp = st.SessFilesUp
	env.SessMinutes = st.SessMinutes
	env.SessTimeLeft = st.SessTimeLeft
	env.NewMsgsTotal = st.NewMsgsTotal
	env.BBSCallsToday = st.BBSCallsToday
	env.BBSUniqueToday = st.BBSUniqueToday
	env.BBSMsgTotal = st.BBSMsgTotal
	env.BBSConfCount = st.BBSConfCount
	env.BBSFileTotal = st.BBSFileTotal
	env.BBSFileToday = st.BBSFileToday
	env.BBSFileMonth = st.BBSFileMonth
}

// runPPE executes a PPL source file (.PPS) in the context of this session.
func (s *session) runPPE(ppsPath string) {
	cfg := config.Get()
	env := ppl.EnvFromSession(
		s.rw,
		s.user.Name, s.user.City,
		s.user.SecurityLevel, s.user.TimesOnline,
		s.nodeID,
		cfg.BBS.Name, cfg.Sysop.Name,
		s.cp437Out,
		s.pplReadLine,
	)
	s.populatePplStats(env)
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusDoor, "PPE: "+ppsPath, s.user.ID, s.user.Name, s.user.City)
	if err := ppl.Run(ppsPath, env); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "PPE error: "+err.Error()))
	}
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusMain, "Main Menu", s.user.ID, s.user.Name, s.user.City)
}

// ── I/O helpers ───────────────────────────────────────────────────────────────

func (s *session) write(text string) {
	_, _ = io.WriteString(s.rw, ansi.EncodeOutput(text, s.cp437Out))
}

func (s *session) writeln(text string) {
	s.write(text + "\r\n")
}

// readline reads a line from the user, echoing characters if we are a Telnet
// connection (echoInput=true). For SSH the PTY handles echo locally.
func (s *session) readline() string {
	return s.readlineEcho(true)
}

// readlineHidden reads a line without echoing characters (for passwords).
func (s *session) readlineHidden() string {
	return s.readlineEcho(false)
}

// readlineEcho is the core byte-by-byte line reader.
// When doEcho is true AND s.echoInput is true, each printable character is
// echoed back to the client; backspace is handled with BS-SPACE-BS.
func (s *session) readlineEcho(doEcho bool) string {
	var buf []byte
	single := make([]byte, 1)
	shouldEcho := doEcho && s.echoInput
	for {
		n, err := s.rw.Read(single)
		if n > 0 {
			b := single[0]
			// CR or LF → end of input; echo CRLF
			if b == '\r' || b == '\n' {
				if shouldEcho {
					_, _ = s.rw.Write([]byte("\r\n"))
				}
				break
			}
			// Backspace (0x08) or DEL (0x7F)
			if b == 0x08 || b == 0x7F {
				if len(buf) > 0 {
					buf = buf[:len(buf)-1]
					if shouldEcho {
						_, _ = s.rw.Write([]byte{0x08, ' ', 0x08}) // erase character on terminal
					}
				}
				continue
			}
			// Ignore other control characters
			if b < 0x20 {
				continue
			}
			buf = append(buf, b)
			if shouldEcho {
				_, _ = s.rw.Write([]byte{b})
			}
		}
		if n > 0 && s.idleTimer != nil {
			// Any incoming byte resets the idle timeout.
			s.idleTimer.Reset(time.Duration(config.Get().Session.IdleTimeoutMins) * time.Minute)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}
