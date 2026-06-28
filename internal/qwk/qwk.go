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
//   v0.7.0  2026-06-26  Phase 1 (VirtAnd/VirtTerm): initial implementation —
//                        real legacy QWK/REP binary packet format (not a
//                        bespoke JSON shortcut), used by the VirtAnd Android
//                        point client for offline message sync.
// ============================================================================

// Package qwk implements the classic QWK offline-mail packet format
// (download) and the REP reply-packet format (upload), as used by FidoNet
// point software and BBS offline readers since the late 1980s.
//
// QWK packet (download), a ZIP archive containing:
//
//	MESSAGES.DAT  — all message text, in fixed 128-byte blocks. Block 0 is
//	                a reserved/unused header block. Every message begins on
//	                a block boundary with a 128-byte header record (see
//	                messageHeader), followed by ceil(len(body)/128) blocks
//	                of body text, soft-wrapped with 0xE3 markers and space-
//	                padded to fill the final block.
//	CONTROL.DAT   — plain text, CRLF-terminated lines: BBS identity, the
//	                caller's name, message counts, then a NAME/NUMBER pair
//	                per included conference.
//	NNN.NDX       — one per conference (NNN = zero-padded conference
//	                number), each a flat array of 4-byte little-endian
//	                1-based block numbers pointing at that conference's
//	                message headers in MESSAGES.DAT.
//	DOOR.ID       — plain text BBS/sysop identification, read by some doors.
//
// REP packet (upload): a ZIP archive containing one flat text file per
// reply, named "<N>.MSG" (N = a sequential reply number), in the simple
// line-oriented REP-message format (see ParseRep) — conference number,
// reference number, to/from/subject header lines, a blank line, then the
// body.
package qwk

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/postname"
	"github.com/virtbbs/virtbbs/internal/users"
)

const blockSize = 128

// softCR is the QWK soft-line-break marker (0xE3) used in MESSAGES.DAT body
// text in place of "\r\n" — readers convert it back to a line break on display.
const softCR = 0xE3

// messageHeader is the fixed 128-byte record preceding every message body
// in MESSAGES.DAT. Field layout per the standard QWK specification.
type messageHeader struct {
	Status    byte   // 1 byte:  ' '=public unread, '-'=private, etc.
	MsgNum    int    // 7 chars: ASCII message number
	Date      string // 8 chars: MM-DD-YY
	Time      string // 5 chars: HH:MM
	To        string // 25 chars
	From      string // 25 chars
	Subject   string // 25 chars
	Password  string // 12 chars (blank)
	RefNum    int    // 8 chars: ASCII reference message number (0 = none)
	NumBlocks int    // 2 chars: ASCII, total 128-byte blocks (header + body)
	Active    byte   // 1 char:  ' '=active, 'E'=deleted
	ConfNum   int    // 7 chars: ASCII conference number
	NetTag    byte   // 1 char:  blank (no FidoNet net-tag support yet)
	_pad      byte   // 1 char:  reserved
}

func leftPadNum(n, width int) string {
	s := strconv.Itoa(n)
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func fixedWidth(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

// encode renders the header as exactly 128 bytes.
func (h *messageHeader) encode() []byte {
	buf := make([]byte, 0, blockSize)
	buf = append(buf, h.Status)
	buf = append(buf, []byte(leftPadNum(h.MsgNum, 7))...)
	buf = append(buf, []byte(fixedWidth(h.Date, 8))...)
	buf = append(buf, []byte(fixedWidth(h.Time, 5))...)
	buf = append(buf, []byte(fixedWidth(h.To, 25))...)
	buf = append(buf, []byte(fixedWidth(h.From, 25))...)
	buf = append(buf, []byte(fixedWidth(h.Subject, 25))...)
	buf = append(buf, []byte(fixedWidth(h.Password, 12))...)
	buf = append(buf, []byte(leftPadNum(h.RefNum, 8))...)
	buf = append(buf, []byte(leftPadNum(h.NumBlocks, 2))...)
	buf = append(buf, h.Active)
	buf = append(buf, []byte(leftPadNum(h.ConfNum, 7))...)
	buf = append(buf, h.NetTag)
	buf = append(buf, ' ') // reserved
	if len(buf) != blockSize {
		panic(fmt.Sprintf("qwk: header encode produced %d bytes, want %d", len(buf), blockSize))
	}
	return buf
}

// decode parses a 128-byte header record.
func decodeHeader(b []byte) (*messageHeader, error) {
	if len(b) != blockSize {
		return nil, fmt.Errorf("qwk: header block must be %d bytes, got %d", blockSize, len(b))
	}
	h := &messageHeader{}
	h.Status = b[0]
	h.MsgNum, _ = strconv.Atoi(strings.TrimSpace(string(b[1:8])))
	h.Date = strings.TrimSpace(string(b[8:16]))
	h.Time = strings.TrimSpace(string(b[16:21]))
	h.To = strings.TrimSpace(string(b[21:46]))
	h.From = strings.TrimSpace(string(b[46:71]))
	h.Subject = strings.TrimSpace(string(b[71:96]))
	h.Password = strings.TrimSpace(string(b[96:108]))
	h.RefNum, _ = strconv.Atoi(strings.TrimSpace(string(b[108:116])))
	h.NumBlocks, _ = strconv.Atoi(strings.TrimSpace(string(b[116:118])))
	h.Active = b[118]
	h.ConfNum, _ = strconv.Atoi(strings.TrimSpace(string(b[119:126])))
	h.NetTag = b[126]
	return h, nil
}

// encodeBody soft-wraps body text into 128-byte blocks using the 0xE3
// line-break marker in place of newlines, space-padding the final block.
func encodeBody(body string) []byte {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	var buf bytes.Buffer
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte(softCR)
	}
	for buf.Len()%blockSize != 0 {
		buf.WriteByte(' ')
	}
	return buf.Bytes()
}

// decodeBody reverses encodeBody, turning 0xE3 markers back into "\r\n" and
// trimming the space-padding from the final block.
func decodeBody(b []byte) string {
	s := strings.ReplaceAll(string(b), string([]byte{softCR}), "\r\n")
	return strings.TrimRight(s, " ")
}

// PacketMeta describes the BBS identity fields written into CONTROL.DAT
// and DOOR.ID.
type PacketMeta struct {
	BBSName   string
	CityState string
	BBSPhone  string
	SysopName string
	BBSID     string // short alphanumeric BBS identifier
}

// confInfo tracks one conference's name and the 1-based MESSAGES.DAT block
// numbers of each message header written for it, for CONTROL.DAT/NDX output.
type confInfo struct {
	ID   int
	Name string
	ndx  []uint32
}

// BuildPacket builds a QWK packet (as a ZIP archive's raw bytes) containing
// every message new since the user's last QWK sync in each of the given
// conferences, sourced from messages.Store.ListFrom / users.Store.GetLastRead.
// On success, the user's last-read marker for each conference is advanced
// to the highest message number included, via users.Store.SetLastRead.
func BuildPacket(meta PacketMeta, userStore *users.Store, msgStore *messages.Store, confStore *conferences.Store, userID int64, conferenceIDs []int) ([]byte, error) {
	caller, err := userStore.GetByID(userID)
	if err != nil {
		return nil, fmt.Errorf("qwk: load user %d: %w", userID, err)
	}

	var messagesDat bytes.Buffer
	// Block 0 is reserved (unused by VirtBBS) — write 128 spaces.
	messagesDat.Write(bytes.Repeat([]byte{' '}, blockSize))

	var confs []*confInfo
	totalMessages := 0

	for _, cid := range conferenceIDs {
		conf, err := confStore.Get(cid)
		if err != nil {
			return nil, fmt.Errorf("qwk: conference %d: %w", cid, err)
		}
		ci := &confInfo{ID: cid, Name: conf.Name}

		lastRead := userStore.GetLastRead(userID, cid)
		msgs, err := msgStore.ListFrom(cid, lastRead+1, 100000)
		if err != nil {
			return nil, fmt.Errorf("qwk: list messages in conference %d: %w", cid, err)
		}

		highWater := lastRead
		for _, m := range msgs {
			bodyBlocks := encodeBody(m.Body)
			numBlocks := 1 + len(bodyBlocks)/blockSize

			blockNum := uint32(messagesDat.Len()/blockSize) + 1 // 1-based
			ci.ndx = append(ci.ndx, blockNum)

			h := &messageHeader{
				Status:    ' ',
				MsgNum:    m.MsgNumber,
				Date:      m.DatePosted.Format("01-02-06"),
				Time:      m.DatePosted.Format("15:04"),
				To:        m.ToName,
				From:      m.FromName,
				Subject:   m.Subject,
				RefNum:    0,
				NumBlocks: numBlocks,
				Active:    ' ',
				ConfNum:   cid,
			}
			messagesDat.Write(h.encode())
			messagesDat.Write(bodyBlocks)

			totalMessages++
			if m.MsgNumber > highWater {
				highWater = m.MsgNumber
			}
		}

		if highWater > lastRead {
			if err := userStore.SetLastRead(userID, cid, highWater); err != nil {
				return nil, fmt.Errorf("qwk: update last-read for conference %d: %w", cid, err)
			}
		}
		confs = append(confs, ci)
	}

	control := buildControlDat(meta, caller.Name, totalMessages, confs)
	doorID := buildDoorID(meta)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	if err := writeZipFile(zw, "MESSAGES.DAT", messagesDat.Bytes()); err != nil {
		return nil, err
	}
	if err := writeZipFile(zw, "CONTROL.DAT", []byte(control)); err != nil {
		return nil, err
	}
	if err := writeZipFile(zw, "DOOR.ID", []byte(doorID)); err != nil {
		return nil, err
	}
	for _, ci := range confs {
		ndxBytes := make([]byte, 0, len(ci.ndx)*4)
		for _, blockNum := range ci.ndx {
			var b [4]byte
			b[0] = byte(blockNum)
			b[1] = byte(blockNum >> 8)
			b[2] = byte(blockNum >> 16)
			b[3] = byte(blockNum >> 24)
			ndxBytes = append(ndxBytes, b[:]...)
		}
		name := fmt.Sprintf("%03d.NDX", ci.ID)
		if err := writeZipFile(zw, name, ndxBytes); err != nil {
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return zipBuf.Bytes(), nil
}

func writeZipFile(zw *zip.Writer, name string, data []byte) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// buildControlDat renders CONTROL.DAT per the standard QWK layout: ten fixed
// identity/count lines, followed by a NAME/NUMBER pair per conference.
func buildControlDat(meta PacketMeta, callerName string, totalMessages int, confs []*confInfo) string {
	var b strings.Builder
	crlf := func(s string) { b.WriteString(s); b.WriteString("\r\n") }

	crlf(meta.BBSName)
	crlf(meta.CityState)
	crlf(meta.BBSPhone)
	crlf(meta.SysopName)
	crlf(fmt.Sprintf("00000,%s", meta.BBSID))
	crlf(time.Now().Format("01-02-2006,15:04:05"))
	crlf(callerName)
	crlf("") // menu name (unused)
	crlf(strconv.Itoa(totalMessages))
	crlf("0")
	for _, ci := range confs {
		crlf(strconv.Itoa(ci.ID))
		crlf(ci.Name)
	}
	crlf("0")
	crlf("")  // welcome screen file name (none)
	crlf("")  // news file name (none)
	crlf("")  // "log off" file name (none)
	return b.String()
}

// buildDoorID renders DOOR.ID, a minimal identification file some QWK
// readers/doors expect alongside CONTROL.DAT.
func buildDoorID(meta PacketMeta) string {
	var b strings.Builder
	crlf := func(s string) { b.WriteString(s); b.WriteString("\r\n") }
	crlf(meta.BBSName)
	crlf(meta.SysopName)
	crlf(meta.BBSPhone)
	crlf("0")
	crlf("0")
	return b.String()
}

// ReplyMsg is one parsed reply from an uploaded REP packet, ready to be
// posted via messages.Store.Post.
type ReplyMsg struct {
	ConferenceID int
	RefNum       int // message number this replies to, 0 if none
	ToName       string
	FromName     string
	Subject      string
	Body         string
}

// ParseRep parses an uploaded REP packet (a ZIP archive of "<N>.MSG" reply
// files) into a slice of ReplyMsg, in numeric filename order.
//
// Each "<N>.MSG" file is a simple line-oriented text format:
//
//	Line 1: conference number
//	Line 2: reference message number (0 = none)
//	Line 3: To
//	Line 4: From
//	Line 5: Subject
//	Line 6: (blank separator)
//	Line 7+: message body, to end of file
func ParseRep(data []byte) ([]*ReplyMsg, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("qwk: not a valid REP zip archive: %w", err)
	}

	var msgFiles []*zip.File
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Ext(f.Name), ".msg") {
			msgFiles = append(msgFiles, f)
		}
	}
	sort.Slice(msgFiles, func(i, j int) bool { return msgFiles[i].Name < msgFiles[j].Name })

	var out []*ReplyMsg
	for _, f := range msgFiles {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("qwk: open %s: %w", f.Name, err)
		}
		raw, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("qwk: read %s: %w", f.Name, err)
		}

		msg, err := parseReplyFile(raw)
		if err != nil {
			return nil, fmt.Errorf("qwk: parse %s: %w", f.Name, err)
		}
		out = append(out, msg)
	}
	return out, nil
}

// filepath.Ext is the only thing we need from path/filepath.

// PostReplies posts each parsed reply via messages.Store.Post using the
// conference echomail FromName policy for the uploading user.
func PostReplies(msgStore *messages.Store, confStore *conferences.Store, u *users.User, replies []*ReplyMsg) (int, error) {
	posted := 0
	for _, r := range replies {
		conf, err := confStore.Get(r.ConferenceID)
		if err != nil {
			return posted, fmt.Errorf("qwk: conference %d: %w", r.ConferenceID, err)
		}
		if err := postname.ValidateEchoPost(conf, u); err != nil {
			return posted, fmt.Errorf("qwk: conference %d: %w", r.ConferenceID, err)
		}
		fromName := postname.ForConference(conf, u)
		m := &messages.Message{
			ConferenceID: r.ConferenceID,
			FromName:     fromName,
			ToName:       r.ToName,
			Subject:      r.Subject,
			Body:         r.Body,
			DatePosted:   time.Now(),
			Echo:         conf != nil && conf.Echo,
		}
		var replyTo *messages.Message
		if r.RefNum > 0 {
			if orig, err := msgStore.Get(r.ConferenceID, r.RefNum); err == nil {
				replyTo = orig
			}
		}
		lang := "en"
		if u != nil && strings.TrimSpace(u.Locale) != "" {
			lang = fido.NormalizeLangCode(u.Locale)
		}
		fido.ApplyLocalEchoMeta(m, conf, postname.EchoOrigAddr(conf), lang, replyTo)
		if err := msgStore.Post(m); err != nil {
			return posted, fmt.Errorf("qwk: post reply to conference %d: %w", r.ConferenceID, err)
		}
		posted++
	}
	return posted, nil
}

func parseReplyFile(raw []byte) (*ReplyMsg, error) {
	text := strings.ReplaceAll(string(raw), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 6 {
		return nil, fmt.Errorf("too few header lines (%d)", len(lines))
	}
	confNum, _ := strconv.Atoi(strings.TrimSpace(lines[0]))
	refNum, _ := strconv.Atoi(strings.TrimSpace(lines[1]))
	m := &ReplyMsg{
		ConferenceID: confNum,
		RefNum:       refNum,
		ToName:       strings.TrimSpace(lines[2]),
		FromName:     strings.TrimSpace(lines[3]),
		Subject:      strings.TrimSpace(lines[4]),
	}
	// lines[5] is the blank separator; body is everything after it.
	if len(lines) > 6 {
		m.Body = strings.Join(lines[6:], "\r\n")
	}
	return m, nil
}
