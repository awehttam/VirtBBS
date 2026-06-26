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
// ============================================================================

// Package session manages a single connected user's BBS session.
package session

import (
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
					_, _ = io.WriteString(rw, ansi.ToCP437(msg))
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
			_, _ = io.WriteString(rw, ansi.ToCP437("\r\n\033[1;31m*** Idle timeout — disconnecting ***\033[0m\r\n"))
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
			_, _ = io.WriteString(rw, ansi.ToCP437("\r\n\033[1;33m*** Your time for this call has expired. Goodbye! ***\033[0m\r\n"))
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
	w := 42
	line := func(content string) string {
		pad := w - 4 - len(content)
		if pad < 0 {
			pad = 0
		}
		return ansi.Bold() + ansi.Color(ansi.BrightCyan) + "║  " +
			ansi.Color(ansi.BrightWhite) + content + strings.Repeat(" ", pad) +
			ansi.Color(ansi.BrightCyan) + "║" + ansi.Reset()
	}
	border := ansi.Bold() + ansi.Color(ansi.BrightCyan)
	s.writeln(border + "╔" + strings.Repeat("═", w-2) + "╗" + ansi.Reset())
	s.writeln(line(cfg.BBS.Name))
	s.writeln(line("Powered by VirtBBS"))
	s.writeln(border + "╚" + strings.Repeat("═", w-2) + "╝" + ansi.Reset())
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
	s.write(ansi.Prompt("Full name: "))
	name := s.readline()
	if name == "" {
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
	secLevel := config.Get().Session.NewUserSecurity
	if secLevel <= 0 {
		secLevel = 10
	}
	u := &users.User{
		Name:          name,
		City:          city,
		SecurityLevel: secLevel,
		PageLength:    24,
		XferProtocol:  "Z",
		ANSI:          true,
	}
	if err := s.deps.Users.Create(u, pass); err != nil {
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
			"  [T]alk       [D]oors   [P]PE           [R]profile [G]oodbye" +
			ansi.Reset())
		if s.user.Sysop {
			s.writeln(ansi.Color(ansi.BrightYellow) + "  [S]ysop menu" + ansi.Reset())
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
			"  [R]ead   [E]nter   [N]ew (since last)" + netmailOpt + "   [Q]uit" +
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
		case "K":
			if fidoEnabled {
				s.netmailCompose()
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
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightCyan) + strings.Repeat("─", 60) + ansi.Reset())
		s.writeln(ansi.Color(ansi.BrightCyan) + fmt.Sprintf("Msg #%-5d", m.MsgNumber) +
			ansi.Color(ansi.White) + fmt.Sprintf("  From: %-20s  To: %s", m.FromName, m.ToName) + ansi.Reset())
		s.writeln(ansi.Color(ansi.Yellow) + "  Subj: " + m.Subject + ansi.Reset())
		s.writeln(ansi.Color(ansi.White) + "  Date: " + m.DatePosted.Format("01-02-2006 15:04") + ansi.Reset())
		s.writeln(ansi.Color(ansi.BrightCyan) + strings.Repeat("─", 60) + ansi.Reset())
		s.writeln(m.Body)

		lastReadNum = m.MsgNumber
		s.statMsgsRead++
		s.write(ansi.Prompt("[N]ext / [R]eply / [Q]uit: "))
		switch strings.ToUpper(strings.TrimSpace(s.readline())) {
		case "Q":
			// Save progress even on early quit.
			if lastReadNum > 0 && s.user != nil {
				_ = s.deps.Users.SetLastRead(s.user.ID, s.conference, lastReadNum)
			}
			return
		case "R":
			s.enterReply(m)
		}
	}
	// Update last-read after finishing all messages.
	if lastReadNum > 0 && s.user != nil {
		_ = s.deps.Users.SetLastRead(s.user.ID, s.conference, lastReadNum)
	}
	s.writeln(ansi.Colorize(ansi.Yellow, "End of messages."))
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

	m := &messages.Message{
		ConferenceID: s.conference,
		FromName:     s.user.Name,
		ToName:       to,
		Subject:      subj,
		Status:       "A",
		Body:         result.Body,
	}
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

	m := &messages.Message{
		ConferenceID: s.conference,
		FromName:     s.user.Name,
		ToName:       orig.FromName,
		Subject:      subj,
		Status:       "A",
		Body:         result.Body,
	}
	if err := s.deps.Messages.Post(m); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error posting reply: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf("Reply #%d posted (%d lines).", m.MsgNumber, result.Lines)))
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
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [L]ist dirs   [B]rowse dir   [D]ownload   [U]pload   [S]earch   [Q]uit" +
			ansi.Reset())
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
	s.statFilesUp++
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
			"  [N]ode list   [K]ick node   [C]onference   [L]og   [F]ido   [Q]uit" +
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
			"  [T]oss inbound   [S]can outbound   [N]odelist   [L]oad nodelist now   [E]cho flags   [P]oll uplink" + ansi.Reset())
		s.writeln(ansi.Color(ansi.BrightYellow) +
			"  [I]Ping a node   [X]Trace a node   [A]reaFix   [F]ileFix   [Q]uit" + ansi.Reset())
		s.write(ansi.Prompt("FidoNet command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "T":
			s.writeln(ansi.Colorize(ansi.White, "Tossing inbound packets (all networks)…"))
			result := fido.TossAll(&cfg.Fido, s.deps.Messages, s.deps.Conferences)
			s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
				"Toss complete: %d packet(s), %d imported, %d skipped.",
				result.Packets, result.Imported, result.Skipped)))
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
		case "Q", "":
			return
		}
	}
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
	s.writeln(ansi.Colorize(ansi.BrightGreen, "Downlink removed and subscriptions cleared."))
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
		s.writeln("")
		s.writeln(ansi.Colorize(ansi.Yellow,
			"  Note: no TIC file-echo distribution pipeline exists yet — subscriptions"))
		s.writeln(ansi.Colorize(ansi.Yellow,
			"  are tracked but nothing currently sends files based on them."))
		s.writeln("")
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

	result := fido.PollAndToss(target, s.deps.Messages, s.deps.Conferences)
	if result.Poll.Error != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Poll error: "+result.Poll.Error.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"Poll complete: sent %d, received %d file(s).",
		len(result.Poll.Sent), len(result.Poll.Received))))

	if result.Toss != nil {
		s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
			"Auto-toss complete: %d packet(s), %d imported, %d skipped.",
			result.Toss.Packets, result.Toss.Imported, result.Toss.Skipped)))
		for _, e := range result.Toss.Errors {
			s.writeln(ansi.Colorize(ansi.Red, "  Toss error: "+e))
		}
	}
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
		FromName: s.user.Name,
		FromAddr: cfg.Fido.Address,
		ToName:   toName,
		ToAddr:   toAddr,
		Subject:  subject,
		Body:     body,
		Crash:    crash,
		Network:  "",
	}

	// Determine routing and write PKT.
	nd := cfg.Fido.AllNetworks()[0]
	nextHop, err := fido.RouteAddr(msg, &nd)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Routing error: "+err.Error()))
		return
	}
	outDir := fido.OutboundDir(nd.OutboundDir, nextHop, crash)
	origAddr, _ := fido.ParseAddr(cfg.Fido.Address)

	pktPath, err := fido.WritePKT(origAddr, nextHop, nd.Password, outDir, []*fido.NetmailMsg{msg})
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Error writing PKT: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, fmt.Sprintf(
		"NetMail queued → %s (next hop: %s)", pktPath, nextHop.String())))
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
		s.writeln(fmt.Sprintf("  %sCity%s       : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.City))
		s.writeln(fmt.Sprintf("  %sANSI%s       : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), yesNo(s.user.ANSI)))
		s.writeln(fmt.Sprintf("  %sEditor%s     : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), editorLabel(s.user.EditorType)))
		s.writeln(fmt.Sprintf("  %sPage length%s: %d", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.PageLength))
		s.writeln(fmt.Sprintf("  %sProtocol%s   : %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), s.user.XferProtocol))
		s.writeln(fmt.Sprintf("  %sExpert mode%s: %s", ansi.Color(ansi.BrightCyan), ansi.Reset(), yesNo(s.user.ExpertMode)))
		s.writeln("")
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [C]ity   [P]assword   [A]NSI   [M]sg editor   [L]ines/page   [X]fer   [E]xpert   [Q]uit" +
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

// runPPE executes a PPL source file (.PPS) in the context of this session.
func (s *session) runPPE(ppsPath string) {
	cfg := config.Get()
	env := ppl.EnvFromSession(
		s.rw,
		s.user.Name, s.user.City,
		s.user.SecurityLevel, s.user.TimesOnline,
		s.nodeID,
		cfg.BBS.Name, cfg.Sysop.Name,
	)
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusDoor, "PPE: "+ppsPath, s.user.ID, s.user.Name, s.user.City)
	if err := ppl.Run(ppsPath, env); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "PPE error: "+err.Error()))
	}
	_ = s.deps.Nodes.Update(s.nodeID, node.StatusMain, "Main Menu", s.user.ID, s.user.Name, s.user.City)
}

// ── I/O helpers ───────────────────────────────────────────────────────────────

func (s *session) write(text string) {
	_, _ = io.WriteString(s.rw, ansi.ToCP437(text))
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
