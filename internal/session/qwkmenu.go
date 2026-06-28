package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/virtbbs/virtbbs/internal/ansi"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/qwk"
	"github.com/virtbbs/virtbbs/internal/transfer"
)

// qwkMenu offers classic QWK/REP offline mail transfer via Zmodem.
func (s *session) qwkMenu() {
	for {
		s.writeln(ansi.Header("Offline Mail (QWK / REP)"))
		s.writeln(ansi.Color(ansi.BrightWhite) +
			"  [D]ownload QWK packet   [U]pload REP replies   [Q]uit" +
			ansi.Reset())
		s.writeln(ansi.Color(ansi.Yellow) +
			"  Use a QWK reader (VirtAnd, Blue Wave, etc.) with Zmodem." +
			ansi.Reset())
		s.write(ansi.Prompt("QWK command: "))
		cmd := strings.ToUpper(strings.TrimSpace(s.readline()))
		switch cmd {
		case "D":
			s.downloadQwkPacket()
		case "U":
			s.uploadRepPacket()
		case "Q", "":
			return
		default:
			s.writeln(ansi.Colorize(ansi.Red, "Unknown command."))
		}
	}
}

func (s *session) downloadQwkPacket() {
	all, err := s.deps.Conferences.List()
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Could not list conferences."))
		return
	}
	var ids []int
	for _, c := range all {
		if s.user.SecurityLevel >= c.ReadSec {
			ids = append(ids, c.ID)
		}
	}
	if len(ids) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No conferences available for QWK download."))
		return
	}
	if s.countNewQwkMessages(ids) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "No new messages since your last QWK download."))
		return
	}

	cfg := config.Get()
	meta := qwk.PacketMeta{
		BBSName:   cfg.BBS.Name,
		SysopName: cfg.Sysop.Name,
		BBSID:     "VBBS",
	}
	data, err := qwk.BuildPacket(meta, s.deps.Users, s.deps.Messages, s.deps.Conferences, s.user.ID, ids)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "QWK build failed: "+err.Error()))
		return
	}

	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '_'
	}, s.user.Name)
	if safeName == "" {
		safeName = "USER"
	}
	filename := safeName + ".QWK"

	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("virtbbs-qwk-%d-%s", os.Getpid(), filename))
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Write error: "+err.Error()))
		return
	}
	defer os.Remove(tmpPath)

	s.writeln(ansi.Colorize(ansi.BrightGreen,
		fmt.Sprintf("Sending %s (%d bytes) via Zmodem...", filename, len(data))))
	s.writeln(ansi.Colorize(ansi.White, "Start your Zmodem receive now. Press Ctrl+X to abort."))

	if err := transfer.SendFile(s.rw, tmpPath); err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "\r\nTransfer failed: "+err.Error()))
		return
	}
	s.writeln(ansi.Colorize(ansi.BrightGreen, "QWK packet sent. Import it in your offline reader."))
}

func (s *session) uploadRepPacket() {
	uploadable := 0
	all, _ := s.deps.Conferences.List()
	for _, c := range all {
		if s.user.SecurityLevel >= c.WriteSec {
			uploadable++
		}
	}
	if uploadable == 0 {
		s.writeln(ansi.Colorize(ansi.Red, "You do not have write access to any conference."))
		return
	}

	destDir, err := os.MkdirTemp("", "virtbbs-rep-*")
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Temp dir error: "+err.Error()))
		return
	}
	defer os.RemoveAll(destDir)

	s.writeln(ansi.Colorize(ansi.BrightGreen, "Ready to receive REP packet via Zmodem."))
	s.writeln(ansi.Colorize(ansi.White, "Start your Zmodem send now. Press Ctrl+X to abort."))

	receivedPath, err := transfer.ReceiveFile(s.rw, destDir)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "\r\nUpload failed: "+err.Error()))
		return
	}

	raw, err := os.ReadFile(receivedPath)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Could not read uploaded file: "+err.Error()))
		return
	}

	replies, err := qwk.ParseRep(raw)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, "Invalid REP packet: "+err.Error()))
		return
	}
	if len(replies) == 0 {
		s.writeln(ansi.Colorize(ansi.Yellow, "REP packet contained no messages."))
		return
	}

	var allowed []*qwk.ReplyMsg
	for _, r := range replies {
		c, err := s.deps.Conferences.Get(r.ConferenceID)
		if err != nil {
			continue
		}
		if s.user.SecurityLevel >= c.WriteSec {
			allowed = append(allowed, r)
		}
	}
	posted, err := qwk.PostReplies(s.deps.Messages, s.deps.Conferences, s.user, allowed)
	if err != nil {
		s.writeln(ansi.Colorize(ansi.Red, fmt.Sprintf("Posted %d, then error: %s", posted, err.Error())))
		return
	}
	rejected := len(replies) - posted
	s.writeln(ansi.Colorize(ansi.BrightGreen,
		fmt.Sprintf("REP upload complete: %d message(s) posted, %d rejected.", posted, rejected)))
	_ = filepath.Base(receivedPath)
}

func (s *session) countNewQwkMessages(conferenceIDs []int) int {
	total := 0
	for _, cid := range conferenceIDs {
		lastRead := s.deps.Users.GetLastRead(s.user.ID, cid)
		msgs, err := s.deps.Messages.ListFrom(cid, lastRead+1, 100000)
		if err != nil {
			continue
		}
		total += len(msgs)
	}
	return total
}
