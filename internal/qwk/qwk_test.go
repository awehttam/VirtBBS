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
//   v0.7.0  2026-06-26  Phase 1 (VirtAnd/VirtTerm): round-trip verification —
//                        build a QWK packet against a temp DB and manually
//                        decode MESSAGES.DAT/CONTROL.DAT/.NDX back out;
//                        hand-craft a REP packet and confirm ParseRep +
//                        PostReplies round-trips correctly via messages.Store.
// ============================================================================

package qwk

import (
	"archive/zip"
	"bytes"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/users"
)

func openTestStores(t *testing.T) (*users.Store, *messages.Store, *conferences.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqlDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	userStore, err := users.Open(sqlDB)
	if err != nil {
		t.Fatalf("users.Open: %v", err)
	}
	msgStore, err := messages.Open(sqlDB)
	if err != nil {
		t.Fatalf("messages.Open: %v", err)
	}
	confStore, err := conferences.Open(sqlDB)
	if err != nil {
		t.Fatalf("conferences.Open: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return userStore, msgStore, confStore
}

func zipFile(t *testing.T, zr *zip.Reader, name string) []byte {
	t.Helper()
	for _, f := range zr.File {
		if strings.EqualFold(f.Name, name) {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			var buf bytes.Buffer
			if _, err := buf.ReadFrom(rc); err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			return buf.Bytes()
		}
	}
	t.Fatalf("zip entry %s not found", name)
	return nil
}

func TestBuildPacketRoundTrip(t *testing.T) {
	userStore, msgStore, confStore := openTestStores(t)

	if err := confStore.Create(&conferences.Conference{ID: 1, Name: "Chat", ReadSec: 10, WriteSec: 10}); err != nil {
		t.Fatalf("create conference: %v", err)
	}

	u := &users.User{Name: "PointUser", SecurityLevel: 20, PageLength: 24, XferProtocol: "Z", ANSI: true}
	if err := userStore.Create(u, "pw123456"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	bodies := []string{
		"Hello from VirtBBS!\r\nThis is line two.",
		"A second message,\r\nwith its own body text.",
	}
	for _, body := range bodies {
		m := &messages.Message{ConferenceID: 1, FromName: "Sysop", ToName: "All", Subject: "Test", Body: body}
		if err := msgStore.Post(m); err != nil {
			t.Fatalf("post message: %v", err)
		}
	}

	meta := PacketMeta{BBSName: "VirtBBS Test", SysopName: "Sysop", BBSID: "VBBS"}
	data, err := BuildPacket(meta, userStore, msgStore, confStore, u.ID, []int{1})
	if err != nil {
		t.Fatalf("BuildPacket: %v", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("not a valid zip: %v", err)
	}

	// CONTROL.DAT should name the conference and report 2 messages.
	control := string(zipFile(t, zr, "CONTROL.DAT"))
	if !strings.Contains(control, "Chat\r\n") {
		t.Errorf("CONTROL.DAT missing conference name: %q", control)
	}
	lines := strings.Split(control, "\r\n")
	if len(lines) < 9 || lines[8] != "2" {
		t.Errorf("CONTROL.DAT message count line = %q, want \"2\"", lines[8])
	}

	// MESSAGES.DAT: block 0 reserved, then 2 messages each with header + body.
	msgsDat := zipFile(t, zr, "MESSAGES.DAT")
	if len(msgsDat)%blockSize != 0 {
		t.Fatalf("MESSAGES.DAT length %d not a multiple of %d", len(msgsDat), blockSize)
	}

	// NDX file should point at exactly 2 header blocks, decodable as headers
	// whose body matches what we posted.
	ndx := zipFile(t, zr, "001.NDX")
	if len(ndx) != 8 {
		t.Fatalf("001.NDX length = %d, want 8 (2 entries x 4 bytes)", len(ndx))
	}
	for i := 0; i < 2; i++ {
		blockNum := uint32(ndx[i*4]) | uint32(ndx[i*4+1])<<8 | uint32(ndx[i*4+2])<<16 | uint32(ndx[i*4+3])<<24
		offset := int(blockNum-1) * blockSize
		header, err := decodeHeader(msgsDat[offset : offset+blockSize])
		if err != nil {
			t.Fatalf("decode header at block %d: %v", blockNum, err)
		}
		if header.ConfNum != 1 {
			t.Errorf("entry %d: ConfNum = %d, want 1", i, header.ConfNum)
		}
		if header.Subject != "Test" {
			t.Errorf("entry %d: Subject = %q, want \"Test\"", i, header.Subject)
		}
		bodyStart := offset + blockSize
		bodyEnd := bodyStart + (header.NumBlocks-1)*blockSize
		// encodeBody terminates every line (including the last) with the
		// soft-CR marker, so the decoded body carries one trailing "\r\n"
		// versus the original — this matches real QWK reader behavior.
		body := strings.TrimSuffix(decodeBody(msgsDat[bodyStart:bodyEnd]), "\r\n")
		if body != bodies[i] {
			t.Errorf("entry %d: body = %q, want %q", i, body, bodies[i])
		}
	}

	// Last-read marker should have advanced past both messages.
	if got := userStore.GetLastRead(u.ID, 1); got != 2 {
		t.Errorf("GetLastRead after sync = %d, want 2", got)
	}
}

func TestParseRepAndPostReplies(t *testing.T) {
	_, msgStore, confStore := openTestStores(t)
	if err := confStore.Create(&conferences.Conference{ID: 1, Name: "Chat", ReadSec: 10, WriteSec: 10}); err != nil {
		t.Fatalf("create conference: %v", err)
	}

	// Hand-craft a REP packet: one reply file, "1.MSG".
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("1.MSG")
	if err != nil {
		t.Fatalf("create zip entry: %v", err)
	}
	replyText := strings.Join([]string{
		strconv.Itoa(1), // conference number
		"0",             // ref num
		"All",           // to
		"ClaimedName",   // from (should be overridden by PostReplies' fromName)
		"Re: Test",      // subject
		"",              // blank separator
		"This is my reply.",
		"Second line.",
	}, "\r\n")
	if _, err := w.Write([]byte(replyText)); err != nil {
		t.Fatalf("write zip entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	replies, err := ParseRep(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseRep: %v", err)
	}
	if len(replies) != 1 {
		t.Fatalf("got %d replies, want 1", len(replies))
	}
	r := replies[0]
	if r.ConferenceID != 1 || r.ToName != "All" || r.Subject != "Re: Test" {
		t.Fatalf("unexpected parsed reply: %+v", r)
	}
	if r.Body != "This is my reply.\r\nSecond line." {
		t.Fatalf("unexpected body: %q", r.Body)
	}

	posted, err := PostReplies(msgStore, "RealUser", replies)
	if err != nil {
		t.Fatalf("PostReplies: %v", err)
	}
	if posted != 1 {
		t.Fatalf("posted = %d, want 1", posted)
	}

	stored, err := msgStore.Get(1, 1)
	if err != nil {
		t.Fatalf("Get posted message: %v", err)
	}
	if stored.FromName != "RealUser" {
		t.Errorf("FromName = %q, want %q (PostReplies must not trust the REP file's From line)", stored.FromName, "RealUser")
	}
	if stored.Body != "This is my reply.\r\nSecond line." {
		t.Errorf("Body = %q", stored.Body)
	}
}
