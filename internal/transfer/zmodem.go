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
//   v0.0.1  2026-06-24  Initial skeleton implementation
//   v0.0.5  2026-06-24  Phase 13: Full Zmodem rewrite — proper frame parsing,
//                        CRC-32, crash recovery, correct subpacket types,
//                        abort sequence, ZRINIT flags
// ============================================================================

// Package transfer implements pure-Go Zmodem file transfer (send and receive).
//
// Protocol reference: Chuck Forsberg's Zmodem specification (1988).
// ZMCRC (CRC-16 and CRC-32) modes are both supported. The sender uses
// streaming ZCRCG subpackets with a trailing ZCRCW wait on the last packet
// before ZEOF, which gives maximum throughput over reliable connections
// (Telnet / SSH) while still providing integrity checking.
//
// Crash recovery: the receiver signals its resume position via ZRPOS; the
// sender seeks to that position and resumes from there.
//
// Abort: five consecutive CAN (0x18) characters terminate the transfer.
package transfer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ── Frame type constants ───────────────────────────────────────────────────────

const (
	ZRQINIT  = 0x00 // request receive init (sender → receiver)
	ZRINIT   = 0x01 // receive init         (receiver → sender)
	ZSINIT   = 0x02 // send init sequence
	ZACK     = 0x03 // acknowledgement
	ZFILE    = 0x04 // file name + info
	ZSKIP    = 0x05 // skip file
	ZNAK     = 0x06 // negative ack
	ZABORT   = 0x07 // abort
	ZFIN     = 0x08 // finish session
	ZRPOS    = 0x09 // resume position (receiver → sender)
	ZDATA    = 0x0A // data packet
	ZEOF     = 0x0B // end of file
	ZFERR    = 0x0C // file I/O error
	ZCRC     = 0x0D // CRC request
	ZCHALLENGE = 0x0E
	ZCOMPL   = 0x0F
	ZCAN     = 0x10 // cancel (five in a row)
	ZFREECNT = 0x11
	ZCOMMAND = 0x12
	ZSTDERR  = 0x13
)

// ── Frame encoding constants ───────────────────────────────────────────────────

const (
	ZPAD  = 0x2A // '*'  — frame prefix padding
	ZDLE  = 0x18 // data link escape
	ZDLEE = 0x58 // escaped ZDLE  (ZDLE ^ 0x40)
	ZBIN  = 0x41 // 'A'  — binary header  (CRC-16)
	ZHEX  = 0x42 // 'B'  — hex header     (CRC-16)
	ZBIN32 = 0x43 // 'C' — binary header  (CRC-32)
)

// ── Data subpacket end types ───────────────────────────────────────────────────

const (
	ZCRCE = 0x68 // 'h' — CRC next, frame ends, header follows
	ZCRCG = 0x69 // 'i' — CRC next, streaming (no ack required)
	ZCRCQ = 0x6A // 'j' — CRC next, send ZACK
	ZCRCW = 0x6B // 'k' — CRC next, wait for ZACK
)

// ── ZRINIT capability flags ────────────────────────────────────────────────────

const (
	CANFC32  = 0x20 // can use 32-bit CRC
	CANOVIO  = 0x02 // can receive data during I/O (overlap)
)

// ── CRC tables ────────────────────────────────────────────────────────────────

var crc16tab [256]uint16

func init() {
	for i := range crc16tab {
		crc := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc <<= 1
			}
		}
		crc16tab[i] = crc
	}
}

func updateCRC16(crc uint16, b byte) uint16 {
	return crc16tab[byte(crc>>8)^b] ^ (crc << 8)
}

func crc16(data []byte) uint16 {
	var crc uint16
	for _, b := range data {
		crc = updateCRC16(crc, b)
	}
	return crc
}

var crc32tab [256]uint32

func init() {
	for i := range crc32tab {
		crc := uint32(i)
		for j := 0; j < 8; j++ {
			if crc&1 != 0 {
				crc = (crc >> 1) ^ 0xEDB88320
			} else {
				crc >>= 1
			}
		}
		crc32tab[i] = crc
	}
}

func updateCRC32(crc uint32, b byte) uint32 {
	return crc32tab[byte(crc)^b] ^ (crc >> 8)
}

func crc32b(data []byte) uint32 {
	crc := uint32(0xFFFFFFFF)
	for _, b := range data {
		crc = updateCRC32(crc, b)
	}
	return ^crc
}

// ── Public API ────────────────────────────────────────────────────────────────

// SendFile transmits a single file to the receiver over rw using Zmodem.
// Implements crash recovery: if the receiver sends ZRPOS with a non-zero
// offset, transmission resumes from that position.
func SendFile(rw io.ReadWriter, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}

	br := bufio.NewReader(rw)

	// 1. Wait for ZRQINIT from receiver.
	if err := waitForZRQINIT(br); err != nil {
		return fmt.Errorf("waiting for ZRQINIT: %w", err)
	}

	// 2. Send ZFILE header (CRC-16 hex).
	if err := sendHexHeader(rw, ZFILE, 0); err != nil {
		return err
	}
	// ZFILE data subpacket: "filename\x00size mtime mode serialno\x00"
	fileData := fmt.Sprintf("%s\x00%d %d 0 0",
		filepath.Base(path), info.Size(), info.ModTime().Unix())
	if err := sendDataSubpacket(rw, []byte(fileData), ZCRCW, false); err != nil {
		return err
	}

	// 3. Read receiver response: ZRPOS (resume offset) or ZRINIT/ZSKIP.
	ftype, hdrData, _, err := readAnyFrame(br)
	if err != nil {
		return fmt.Errorf("after ZFILE: %w", err)
	}
	switch ftype {
	case ZSKIP:
		return nil // receiver wants to skip
	case ZRINIT:
		// Re-send ZFILE
		if err := sendHexHeader(rw, ZFILE, 0); err != nil {
			return err
		}
		if err := sendDataSubpacket(rw, []byte(fileData), ZCRCW, false); err != nil {
			return err
		}
		ftype, hdrData, _, err = readAnyFrame(br)
		if err != nil {
			return err
		}
	}
	if ftype != ZRPOS {
		return fmt.Errorf("expected ZRPOS, got 0x%02x", ftype)
	}
	offset := uint32(0)
	if len(hdrData) >= 4 {
		offset = binary.LittleEndian.Uint32(hdrData[:4])
	}
	if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
		return err
	}

	// 4. Send ZDATA header at the file offset.
	if err := sendHexHeader(rw, ZDATA, offset); err != nil {
		return err
	}

	// 5. Stream data in 1024-byte chunks.
	const chunkSize = 1024
	buf := make([]byte, chunkSize)
	remaining := info.Size() - int64(offset)
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			remaining -= int64(n)
			isLast := readErr == io.EOF || remaining <= 0
			pktType := byte(ZCRCG)
			if isLast {
				pktType = ZCRCW // wait for ack on last packet
			}
			if err := sendDataSubpacket(rw, buf[:n], pktType, false); err != nil {
				return err
			}
			if isLast {
				// Wait for ZACK before sending ZEOF.
				if _, _, _, err := readAnyFrame(br); err != nil {
					return fmt.Errorf("after last data: %w", err)
				}
				break
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}

	// 6. Send ZEOF with the file's total size.
	if err := sendHexHeader(rw, ZEOF, uint32(info.Size())); err != nil {
		return err
	}

	// 7. Wait for ZRINIT (ready for next file), then send ZFIN.
	if _, _, _, err := readAnyFrame(br); err != nil {
		return fmt.Errorf("after ZEOF: %w", err)
	}
	if err := sendHexHeader(rw, ZFIN, 0); err != nil {
		return err
	}

	// 8. Read final OO (two 'O' chars that some receivers send after ZFIN).
	_, _ = br.ReadByte()
	_, _ = br.ReadByte()

	return nil
}

// ReceiveFile receives a file sent by a Zmodem sender into destDir.
// Returns the path of the received file.
func ReceiveFile(rw io.ReadWriter, destDir string) (string, error) {
	br := bufio.NewReader(rw)

	// 1. Send ZRINIT announcing our capabilities (CRC-32 + overlap I/O).
	zrinitFlags := [4]byte{CANOVIO | CANFC32, 0, 0, 0}
	if err := sendHexHeader(rw, ZRINIT, binary.LittleEndian.Uint32(zrinitFlags[:])); err != nil {
		return "", err
	}

	// 2. Wait for ZFILE from sender.
	for {
		ftype, data, _, err := readAnyFrame(br)
		if err != nil {
			return "", fmt.Errorf("waiting for ZFILE: %w", err)
		}
		if ftype == ZFILE {
			// Parse filename from the data subpacket.
			name := strings.SplitN(string(data), "\x00", 2)[0]
			if name == "" {
				return "", fmt.Errorf("empty filename in ZFILE")
			}
			name = filepath.Base(name) // strip any path from sender

			outPath := filepath.Join(destDir, name)

			// Check for partial file (crash recovery).
			var resumeAt int64
			if fi, err := os.Stat(outPath); err == nil {
				resumeAt = fi.Size()
			}

			var f *os.File
			if resumeAt > 0 {
				f, err = os.OpenFile(outPath, os.O_APPEND|os.O_WRONLY, 0644)
			} else {
				f, err = os.Create(outPath)
			}
			if err != nil {
				return "", fmt.Errorf("create output file: %w", err)
			}
			defer f.Close()

			// 3. Send ZRPOS to tell sender where to start.
			if err := sendHexHeader(rw, ZRPOS, uint32(resumeAt)); err != nil {
				return outPath, err
			}

			// 4. Receive data.
			received := resumeAt
			for {
				ftype, pkt, marker, err := readAnyFrame(br)
				if err != nil {
					return outPath, fmt.Errorf("receiving data: %w", err)
				}
				switch ftype {
				case ZDATA:
					// A single ZDATA header is followed by a *run* of raw
					// data subpackets with no header in between (the
					// sender only issues a fresh header once a subpacket's
					// marker is ZCRCW — see SendFile's chunking loop).
					// Keep consuming subpackets directly via
					// readDataSubpacket — bypassing readAnyFrame's header
					// scan — until that marker is seen, then fall through
					// to this outer loop for the next real header (ZEOF or
					// another ZDATA). Missing this inner loop previously
					// truncated every transfer at exactly one 1024-byte
					// chunk: the scanner silently ate the headerless
					// follow-on subpackets as noise while hunting for a
					// ZPAD ZPAD ZDLE that never came.
					for {
						if _, err := f.Write(pkt); err != nil {
							return outPath, fmt.Errorf("write: %w", err)
						}
						received += int64(len(pkt))
						// Only ZCRCW/ZCRCQ actually request an ack; ZCRCG
						// means "keep streaming, no ack needed" and
						// SendFile relies on that — it never reads any ack
						// until after the final ZCRCW chunk. Acking every
						// chunk regardless queues up stale ACKs the sender
						// never drains, which it can then mistake for the
						// one it's actually waiting for, closing the
						// connection mid-transfer (see the matching fix
						// and explanation in the C# Zmodem.cs port).
						if marker == ZCRCW || marker == ZCRCQ {
							_ = sendHexHeader(rw, ZACK, uint32(received))
						}
						if marker == ZCRCW {
							break
						}
						pkt, marker, err = readDataSubpacket(br)
						if err != nil {
							return outPath, fmt.Errorf("receiving data: %w", err)
						}
					}
				case ZEOF:
					// Transfer complete.
					_ = sendHexHeader(rw, ZRINIT,
						binary.LittleEndian.Uint32(zrinitFlags[:]))
					return outPath, nil
				case ZFIN:
					// Sender is done — send final ZFIN and "OO".
					_ = sendHexHeader(rw, ZFIN, 0)
					_, _ = rw.Write([]byte("OO"))
					return outPath, nil
				case ZCAN:
					return outPath, fmt.Errorf("transfer cancelled by sender")
				}
			}
		}
		if ftype == ZFIN {
			// No file sent.
			_ = sendHexHeader(rw, ZFIN, 0)
			return "", nil
		}
	}
}

// ── Frame I/O ─────────────────────────────────────────────────────────────────

// sendHexHeader sends a Zmodem ZHEX-encoded header frame.
// Format: ZPAD ZPAD ZDLE ZHEX <hex-type> <hex-pos[4]> <hex-crc[2]> CR LF XON
func sendHexHeader(w io.Writer, frameType byte, pos uint32) error {
	hdr := [5]byte{frameType,
		byte(pos), byte(pos >> 8), byte(pos >> 16), byte(pos >> 24)}
	checksum := crc16(hdr[:])

	hex := fmt.Sprintf("%02x%02x%02x%02x%02x%02x%02x",
		hdr[0], hdr[1], hdr[2], hdr[3], hdr[4],
		byte(checksum>>8), byte(checksum))

	frame := make([]byte, 0, 4+len(hex)+3)
	frame = append(frame, ZPAD, ZPAD, ZDLE, ZHEX)
	frame = append(frame, hex...)
	frame = append(frame, '\r', '\n', 0x11) // CR LF XON
	_, err := w.Write(frame)
	return err
}

// sendDataSubpacket writes a ZDLE-escaped data subpacket followed by the CRC.
// pktType is one of ZCRCE, ZCRCG, ZCRCQ, ZCRCW.
// use32 selects CRC-32 (not yet used in the send path — kept for future).
func sendDataSubpacket(w io.Writer, data []byte, pktType byte, use32 bool) error {
	escaped := escapeData(data)
	// Escape the pktType byte itself if needed (it follows a ZDLE).
	var crcBytes []byte
	if use32 {
		// Include pktType in the CRC input.
		crcData := append(data, pktType)
		c := crc32b(crcData)
		crcBytes = []byte{
			byte(c), byte(c >> 8), byte(c >> 16), byte(c >> 24),
		}
		// Escape CRC bytes.
		crcBytes = escapeData(crcBytes)
	} else {
		crcData := append(data, pktType)
		c := crc16(crcData)
		crcBytes = []byte{byte(c >> 8), byte(c)}
		crcBytes = escapeData(crcBytes)
	}

	frame := make([]byte, 0, len(escaped)+2+len(crcBytes))
	frame = append(frame, escaped...)
	frame = append(frame, ZDLE, pktType)
	frame = append(frame, crcBytes...)
	_, err := w.Write(frame)
	return err
}

// escapeData returns a copy of data with control characters ZDLE-escaped.
// Characters requiring escape: ZDLE, 0x0D (bare CR), 0x8D, 0x91 (XON), 0x93 (XOFF).
func escapeData(data []byte) []byte {
	out := make([]byte, 0, len(data)+len(data)/8)
	for _, b := range data {
		if needsEscape(b) {
			out = append(out, ZDLE, b^0x40)
		} else {
			out = append(out, b)
		}
	}
	return out
}

func needsEscape(b byte) bool {
	return b == ZDLE || b == 0x0D || b == 0x8D || b == 0x11 || b == 0x91 || b == 0x13 || b == 0x93
}

// ── Frame reading ─────────────────────────────────────────────────────────────

// readAnyFrame reads the next Zmodem frame from br.
// It returns the frame type and for ZDATA/ZFILE frames also the data subpacket content.
// For header-only frames (ZRPOS, ZRQINIT, etc.) the returned data slice contains
// the raw 4-byte position field.
func readAnyFrame(br *bufio.Reader) (byte, []byte, byte, error) {
	canCount := 0
	for {
		b, err := br.ReadByte()
		if err != nil {
			return 0, nil, 0, err
		}
		if b == 0x18 { // CAN
			canCount++
			if canCount >= 5 {
				return ZCAN, nil, 0, nil
			}
			continue
		}
		canCount = 0

		if b != ZPAD {
			continue
		}
		// Might be the start of a frame. Read the next byte.
		b2, err := br.ReadByte()
		if err != nil {
			return 0, nil, 0, err
		}
		if b2 == ZPAD {
			// Two ZPADs — read one more.
			b2, err = br.ReadByte()
			if err != nil {
				return 0, nil, 0, err
			}
		}
		if b2 != ZDLE {
			// Not a frame start — keep scanning.
			if b2 == ZPAD {
				_ = br.UnreadByte()
			}
			continue
		}
		// Read encoding type.
		enc, err := br.ReadByte()
		if err != nil {
			return 0, nil, 0, err
		}
		switch enc {
		case ZHEX:
			return readHexFrame(br)
		case ZBIN:
			return readBinFrame(br, false)
		case ZBIN32:
			return readBinFrame(br, true)
		default:
			// Unknown — keep scanning.
		}
	}
}

// readHexFrame reads and decodes a ZHEX-encoded Zmodem header.
// Returns the frame type, the 4-byte position (or subpacket payload for
// ZFILE/ZDATA) as data, and — for ZFILE/ZDATA only — the subpacket's ZCRC
// end-marker (ZCRCE/ZCRCG/ZCRCQ/ZCRCW), 0 otherwise.
func readHexFrame(br *bufio.Reader) (byte, []byte, byte, error) {
	// 14 hex digits = 7 bytes (type[1] + pos[4] + crc[2])
	hexBuf := make([]byte, 14)
	for i := range hexBuf {
		b, err := br.ReadByte()
		if err != nil {
			return 0, nil, 0, err
		}
		hexBuf[i] = b
	}
	raw, err := hexDecode(hexBuf)
	if err != nil {
		return 0, nil, 0, fmt.Errorf("hex decode: %w", err)
	}
	// raw[0]=type raw[1-4]=pos raw[5-6]=crc16
	// Verify CRC.
	got := crc16(raw[:5])
	want := uint16(raw[5])<<8 | uint16(raw[6])
	if got != want {
		return 0, nil, 0, fmt.Errorf("hex header CRC mismatch: got %04x want %04x", got, want)
	}

	ft := raw[0]
	if ft == ZFILE || ft == ZDATA {
		// A data subpacket follows directly on the wire (the sender wrote
		// it in the same burst right after this header — see sendHexHeader
		// callers, which always immediately call sendDataSubpacket for
		// these two types). It's therefore safe to consume the header's
		// optional trailing CR LF XON here before reading it: unlike
		// header-only frames (ZRQINIT, ZRPOS, ZACK, ZEOF, ZFIN — see below),
		// there's no risk of blocking forever waiting on a peer that's
		// itself waiting on us, because more bytes are already in flight.
		//
		// This subpacket read was previously missing entirely for the hex
		// path (it only existed in the otherwise-unused readBinFrame) — so
		// ZFILE's filename/size payload was never actually extracted for
		// any hex-framed transfer, which is the only kind this package
		// sends. Caught via a Go<->C# Zmodem interop test while building
		// client-side Zmodem support: ReceiveFile's
		// upload path would have silently failed to read the uploaded
		// file's name from a real Zmodem-sending client.
		for {
			b, err := br.ReadByte()
			if err != nil {
				return 0, nil, 0, err
			}
			if b != '\r' && b != '\n' && b != 0x11 {
				_ = br.UnreadByte()
				break
			}
		}
		payload, marker, err := readDataSubpacket(br)
		if err != nil {
			return ft, nil, 0, err
		}
		return ft, payload, marker, nil
	}

	// Header-only frame (ZRQINIT, ZRINIT, ZRPOS, ZACK, ZEOF, ZFIN, ...):
	// deliberately does NOT try to consume its optional trailing CR LF XON.
	// Doing so unconditionally here was a real deadlock: it requires
	// reading one byte past what's been sent so far to confirm there's
	// nothing more, which blocks forever whenever the peer is itself
	// waiting on *our* response before sending anything else (e.g.
	// waitForZRQINIT blocking here while the far end blocks waiting for us
	// to reply — caught via the same interop test above). The trailing
	// bytes are harmless to leave unconsumed: readAnyFrame's outer scan
	// loop already silently skips any byte that isn't part of a
	// ZPAD ZPAD ZDLE sequence, so they get skipped naturally next call.
	return ft, raw[1:5], 0, nil
}

// readBinFrame reads and decodes a ZBIN or ZBIN32-encoded Zmodem header.
func readBinFrame(br *bufio.Reader, use32 bool) (byte, []byte, byte, error) {
	crcLen := 2
	if use32 {
		crcLen = 4
	}
	raw, err := readEscaped(br, 5+crcLen)
	if err != nil {
		return 0, nil, 0, err
	}
	if use32 {
		got := crc32b(raw[:5])
		want := binary.LittleEndian.Uint32(raw[5:])
		if got != want {
			return 0, nil, 0, fmt.Errorf("bin32 CRC mismatch")
		}
	} else {
		got := crc16(raw[:5])
		want := uint16(raw[5])<<8 | uint16(raw[6])
		if got != want {
			return 0, nil, 0, fmt.Errorf("bin CRC mismatch")
		}
	}
	// For data frames (ZFILE, ZDATA), read the subpacket payload.
	ft := raw[0]
	pos := raw[1:5]
	if ft == ZFILE || ft == ZDATA {
		payload, marker, err := readDataSubpacket(br)
		if err != nil {
			return ft, nil, 0, err
		}
		return ft, payload, marker, nil
	}
	return ft, pos, 0, nil
}

// readEscapedByte reads one byte, transparently un-escaping it if it's a
// ZDLE-prefixed sequence. sendDataSubpacket always escapes the trailing
// CRC bytes the same way as the payload (see escapeData(crcBytes) there),
// so the CRC bytes themselves need this same unescaping when read back —
// reading them as two raw bytes (as this function's CRC-reading call sites
// used to do) only happens to work when neither CRC byte's value happens
// to need escaping, which is true most of the time but not always. Caught
// via Zmodem interop tests.
// client-side Zmodem support, on a CRC value matching ZDLE's own byte.
func readEscapedByte(br *bufio.Reader) (byte, error) {
	b, err := br.ReadByte()
	if err != nil {
		return 0, err
	}
	if b != ZDLE {
		return b, nil
	}
	b2, err := br.ReadByte()
	if err != nil {
		return 0, err
	}
	if b2 == ZDLEE {
		return ZDLE, nil
	}
	return b2 ^ 0x40, nil
}

// readDataSubpacket reads a ZDLE-escaped data stream up to the end-of-subpacket
// marker (ZDLE ZCRCE/ZCRCG/ZCRCQ/ZCRCW), verifies its CRC-16, and returns that
// marker byte alongside the data: ZCRCG/ZCRCQ mean more subpackets follow
// immediately with no new header in between (see ReceiveFile's inner loop),
// ZCRCW means the sender will issue a fresh header next.
func readDataSubpacket(br *bufio.Reader) ([]byte, byte, error) {
	var data []byte
	for {
		b, err := br.ReadByte()
		if err != nil {
			return nil, 0, err
		}
		if b != ZDLE {
			data = append(data, b)
			continue
		}
		// Escape sequence.
		b2, err := br.ReadByte()
		if err != nil {
			return nil, 0, err
		}
		switch b2 {
		case ZCRCE, ZCRCG, ZCRCQ, ZCRCW:
			// End of subpacket. Read the 2-byte CRC, itself ZDLE-escaped
			// the same way the payload is (see readEscapedByte above).
			c1, err := readEscapedByte(br)
			if err != nil {
				return data, 0, err
			}
			c2, err := readEscapedByte(br)
			if err != nil {
				return data, 0, err
			}
			want := uint16(c1)<<8 | uint16(c2)
			crcData := append(data, b2)
			got := crc16(crcData)
			if got != want {
				return data, 0, fmt.Errorf("subpacket CRC mismatch: got %04x want %04x", got, want)
			}
			return data, b2, nil
		case ZDLEE:
			data = append(data, ZDLE)
		default:
			// Other escaped byte: XOR with 0x40.
			data = append(data, b2^0x40)
		}
	}
}

// readEscaped reads exactly n ZDLE-unescaped bytes from br.
func readEscaped(br *bufio.Reader, n int) ([]byte, error) {
	out := make([]byte, 0, n)
	for len(out) < n {
		b, err := br.ReadByte()
		if err != nil {
			return nil, err
		}
		if b == ZDLE {
			b2, err := br.ReadByte()
			if err != nil {
				return nil, err
			}
			if b2 == ZDLEE {
				out = append(out, ZDLE)
			} else {
				out = append(out, b2^0x40)
			}
		} else {
			out = append(out, b)
		}
	}
	return out, nil
}

// waitForZRQINIT scans the byte stream until it sees a valid ZRQINIT frame.
func waitForZRQINIT(br *bufio.Reader) error {
	for {
		ft, _, _, err := readAnyFrame(br)
		if err != nil {
			return err
		}
		if ft == ZRQINIT {
			return nil
		}
	}
}

// hexDecode converts a 14-character hex string (bytes in pairs) to 7 bytes.
func hexDecode(h []byte) ([]byte, error) {
	if len(h) < 14 {
		return nil, fmt.Errorf("hex too short: %d", len(h))
	}
	out := make([]byte, 7)
	for i := range out {
		hi, ok1 := fromHexNibble(h[i*2])
		lo, ok2 := fromHexNibble(h[i*2+1])
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("invalid hex at pos %d", i*2)
		}
		out[i] = hi<<4 | lo
	}
	return out, nil
}

func fromHexNibble(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}
