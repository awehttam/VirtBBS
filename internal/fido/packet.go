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
//   v0.0.3  2026-06-24  Phase 9: FTS-0001 .PKT reader/writer
//   v0.1.0  2026-06-25  Add Parse() — unified kludge/AREA/SEEN-BY/PATH extraction
//                        replacing the separate AreaTag()/CleanBody() helpers
// ============================================================================

package fido

// FTS-0001 Type-2 Packet Format
// Reference: FidoNet Technical Standard FTS-0001 (1987-09-30)
//
// Packet header: 58 bytes
// Per-message: fixed prefix + null-terminated strings + null-terminated body
// Packet terminator: 2 null bytes

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"
)

// PacketHeader is the 58-byte FTS-0001 packet header.
type PacketHeader struct {
	OrigNode uint16
	DestNode uint16
	Year     uint16
	Month    uint16 // 0-based (0=January)
	Day      uint16
	Hour     uint16
	Minute   uint16
	Second   uint16
	Baud     uint16
	PktType  uint16   // must be 2
	OrigNet  uint16
	DestNet  uint16
	ProdCode byte
	SerialNo byte
	Password [8]byte
	OrigZone uint16
	DestZone uint16
	_        [20]byte // filler / auxNet
}

// Message is a single FTS-0001 message extracted from a .PKT file.
type Message struct {
	OrigAddr Addr
	DestAddr Addr
	DateTime string // raw date/time string from packet
	ToName   string
	FromName string
	Subject  string
	Body     string // full body including AREA: line for echomail
	Attrib   uint16 // attribute flags (e.g. Private 0x0002, Crash 0x0100)
	Cost     uint16

	// Derived fields set by the toss step:
	Area     string // echomail area tag (from "AREA: <tag>" kludge line)
	IsEcho   bool   // true if body contains AREA: kludge
}

// Attribute flag bits for Message.Attrib (FTS-0001 §3.6).
const (
	AttribPrivate = 0x0002
	AttribCrash   = 0x0100
)

// AreaTag returns the echomail area tag from the message body, or "" for netmail.
//
// Deprecated: use Parse().AreaTag, which extracts all metadata in one pass.
func (m *Message) AreaTag() string {
	return m.Parse().AreaTag
}

// CleanBody returns the message body with kludge lines (^A...), the AREA:
// line, and SEEN-BY: lines removed, leaving only the text a reader should
// see — including the tear line ("--- ...") and Origin line ("* Origin:
// ..."), which are NOT kludges and are conventionally shown to end users.
//
// Deprecated: use Parse().Text, which extracts all metadata in one pass.
func (m *Message) CleanBody() string {
	return m.Parse().Text
}

// ParsedBody holds everything extracted from a raw FTS-0001 message body:
// the AREA: tag, FidoNet metadata kludges/lines, and the clean reader-facing
// text (kludges, AREA:, and SEEN-BY: stripped; tear/Origin lines kept).
type ParsedBody struct {
	AreaTag string   // echomail area tag from "AREA:<tag>", "" for netmail
	MSGID   string   // value of the ^AMSGID kludge, "" if absent
	REPLY   string   // value of the ^AREPLY kludge (parent MSGID), "" if absent
	SeenBy  []string // net/node tokens collected from all SEEN-BY: lines
	Path    []string // net/node tokens collected from the ^APATH kludge
	Kludges string   // remaining ^A kludge lines (TZUTC, INTL, etc.), joined with \r
	Text    string   // reader-facing body (kludges/AREA/SEEN-BY stripped)
}

// Parse extracts the AREA: tag, MSGID/REPLY/PATH FidoNet metadata, and SEEN-BY
// node list from the raw message body in a single pass, returning the
// remaining reader-facing text separately. Kludge lines (^A-prefixed),
// the AREA: line, and SEEN-BY: lines are removed from Text; the tear line
// and Origin line are left in Text since real FidoNet readers show them.
func (m *Message) Parse() ParsedBody {
	var pb ParsedBody
	var text, kludges []string

	for _, line := range strings.Split(m.Body, "\r") {
		line = strings.TrimRight(line, "\n")

		switch {
		case strings.HasPrefix(line, "\x01MSGID:"):
			pb.MSGID = strings.TrimSpace(strings.TrimPrefix(line, "\x01MSGID:"))
		case strings.HasPrefix(line, "\x01REPLY:"):
			pb.REPLY = strings.TrimSpace(strings.TrimPrefix(line, "\x01REPLY:"))
		case strings.HasPrefix(line, "\x01PATH:"):
			pb.Path = append(pb.Path, strings.Fields(strings.TrimPrefix(line, "\x01PATH:"))...)
		case strings.HasPrefix(line, "\x01"):
			kludges = append(kludges, line)
		case strings.HasPrefix(line, "AREA:"):
			pb.AreaTag = strings.TrimSpace(strings.TrimPrefix(line, "AREA:"))
		case strings.HasPrefix(line, "SEEN-BY:"):
			pb.SeenBy = append(pb.SeenBy, strings.Fields(strings.TrimPrefix(line, "SEEN-BY:"))...)
		default:
			text = append(text, line)
		}
	}

	pb.Kludges = strings.Join(kludges, "\r")
	pb.Text = strings.Join(text, "\r\n")
	return pb
}

// ── Reader ────────────────────────────────────────────────────────────────────

// ReadPacket reads all messages from an FTS-0001 .PKT reader.
func ReadPacket(r io.Reader) ([]*Message, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read packet: %w", err)
	}

	if len(data) < 58 {
		return nil, fmt.Errorf("packet too short (%d bytes)", len(data))
	}

	// Verify packet type word.
	pktType := binary.LittleEndian.Uint16(data[18:20])
	if pktType != 2 {
		return nil, fmt.Errorf("unsupported packet type %d (expected 2)", pktType)
	}

	origNode := binary.LittleEndian.Uint16(data[0:2])
	destNode := binary.LittleEndian.Uint16(data[2:4])
	origNet  := binary.LittleEndian.Uint16(data[20:22])
	destNet  := binary.LittleEndian.Uint16(data[22:24])
	origZone := binary.LittleEndian.Uint16(data[34:36])
	destZone := binary.LittleEndian.Uint16(data[36:38])

	pktOrig := Addr{Zone: int(origZone), Net: int(origNet), Node: int(origNode)}
	pktDest := Addr{Zone: int(destZone), Net: int(destNet), Node: int(destNode)}

	pos := 58 // skip header
	var msgs []*Message

	for pos < len(data) {
		// Check for packet terminator (2 null bytes).
		if pos+2 <= len(data) && data[pos] == 0 && data[pos+1] == 0 {
			break
		}

		if pos+14 > len(data) {
			break
		}

		// Per-message 2-byte type must be 2.
		msgType := binary.LittleEndian.Uint16(data[pos : pos+2])
		if msgType != 2 {
			break // corrupt/end
		}
		msgOrigNode := binary.LittleEndian.Uint16(data[pos+2 : pos+4])
		msgDestNode := binary.LittleEndian.Uint16(data[pos+4 : pos+6])
		msgOrigNet  := binary.LittleEndian.Uint16(data[pos+6 : pos+8])
		msgDestNet  := binary.LittleEndian.Uint16(data[pos+8 : pos+10])
		attrib := binary.LittleEndian.Uint16(data[pos+10 : pos+12])
		cost := binary.LittleEndian.Uint16(data[pos+12 : pos+14])
		pos += 14

		// Date/time: fixed 20-byte ASCII field, NUL-padded.
		if pos+20 > len(data) {
			break
		}
		dateTime := readFixedStr(data[pos : pos+20])
		pos += 20

		// Read null-terminated strings: toName, fromName, subject, body.
		toName, adv := readNullStr(data[pos:])
		pos += adv
		fromName, adv := readNullStr(data[pos:])
		pos += adv
		subject, adv := readNullStr(data[pos:])
		pos += adv
		body, adv := readNullStr(data[pos:])
		pos += adv

		origAddr := Addr{
			Zone: int(origZone), // inherit from packet when 0
			Net:  int(msgOrigNet),
			Node: int(msgOrigNode),
		}
		if origAddr.Net == 0 {
			origAddr = pktOrig
		}
		destAddr := Addr{
			Zone: int(destZone),
			Net:  int(msgDestNet),
			Node: int(msgDestNode),
		}
		if destAddr.Net == 0 {
			destAddr = pktDest
		}
		_ = pktDest

		msg := &Message{
			OrigAddr: origAddr,
			DestAddr: destAddr,
			DateTime: dateTime,
			ToName:   toName,
			FromName: fromName,
			Subject:  subject,
			Body:     body,
			Attrib:   attrib,
			Cost:     cost,
		}
		msg.Area = msg.AreaTag()
		msg.IsEcho = msg.Area != ""
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// readNullStr reads bytes from b until a null byte, returning the string and
// the number of bytes consumed (including the null).
func readNullStr(b []byte) (string, int) {
	idx := bytes.IndexByte(b, 0)
	if idx < 0 {
		return string(b), len(b)
	}
	return string(b[:idx]), idx + 1
}

// readFixedStr reads a fixed-width ASCII field, truncating at the first NUL
// (the field is NUL-padded if the string is shorter than the field width).
func readFixedStr(b []byte) string {
	idx := bytes.IndexByte(b, 0)
	if idx < 0 {
		return string(b)
	}
	return string(b[:idx])
}

// ── Writer ────────────────────────────────────────────────────────────────────

// WritePacket writes a FTS-0001 .PKT to w containing the given messages.
// orig and dest are the packet-level addresses.
func WritePacket(w io.Writer, orig, dest Addr, password string, msgs []*Message) error {
	// Build packet header.
	hdr := make([]byte, 58)
	now := time.Now()

	binary.LittleEndian.PutUint16(hdr[0:2], uint16(orig.Node))
	binary.LittleEndian.PutUint16(hdr[2:4], uint16(dest.Node))
	binary.LittleEndian.PutUint16(hdr[4:6], uint16(now.Year()))
	binary.LittleEndian.PutUint16(hdr[6:8], uint16(now.Month()-1))
	binary.LittleEndian.PutUint16(hdr[8:10], uint16(now.Day()))
	binary.LittleEndian.PutUint16(hdr[10:12], uint16(now.Hour()))
	binary.LittleEndian.PutUint16(hdr[12:14], uint16(now.Minute()))
	binary.LittleEndian.PutUint16(hdr[14:16], uint16(now.Second()))
	binary.LittleEndian.PutUint16(hdr[16:18], 0)    // baud
	binary.LittleEndian.PutUint16(hdr[18:20], 2)    // packet type
	binary.LittleEndian.PutUint16(hdr[20:22], uint16(orig.Net))
	binary.LittleEndian.PutUint16(hdr[22:24], uint16(dest.Net))
	copy(hdr[26:34], []byte(password)) // password (8 bytes, null-padded)
	binary.LittleEndian.PutUint16(hdr[34:36], uint16(orig.Zone))
	binary.LittleEndian.PutUint16(hdr[36:38], uint16(dest.Zone))

	if _, err := w.Write(hdr); err != nil {
		return err
	}

	// Write each message.
	for _, m := range msgs {
		msgPrefix := make([]byte, 14)
		binary.LittleEndian.PutUint16(msgPrefix[0:2], 2)
		binary.LittleEndian.PutUint16(msgPrefix[2:4], uint16(m.OrigAddr.Node))
		binary.LittleEndian.PutUint16(msgPrefix[4:6], uint16(m.DestAddr.Node))
		binary.LittleEndian.PutUint16(msgPrefix[6:8], uint16(m.OrigAddr.Net))
		binary.LittleEndian.PutUint16(msgPrefix[8:10], uint16(m.DestAddr.Net))
		binary.LittleEndian.PutUint16(msgPrefix[10:12], m.Attrib)
		binary.LittleEndian.PutUint16(msgPrefix[12:14], m.Cost)
		if _, err := w.Write(msgPrefix); err != nil {
			return err
		}

		dateStr := m.DateTime
		if dateStr == "" {
			dateStr = time.Now().Format("02 Jan 06  15:04:05")
		}
		dateBuf := make([]byte, 20)
		copy(dateBuf, dateStr)
		if _, err := w.Write(dateBuf); err != nil {
			return err
		}

		for _, s := range []string{m.ToName, m.FromName, m.Subject, m.Body} {
			if _, err := w.Write(append([]byte(s), 0)); err != nil {
				return err
			}
		}
	}

	// Packet terminator: 2 null bytes.
	_, err := w.Write([]byte{0, 0})
	return err
}
