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
//   v0.0.6  2026-06-24  Initial implementation — BinkP TCP client (RFC draft-ietf-fido-binkp)
// ============================================================================

// Package fido — binkp.go
//
// BinkP TCP client.  Implements enough of the BinkP/1.1 protocol to:
//   - Connect to an uplink and authenticate with M_ADR / M_PWD
//   - Send outbound PKT/ARQ bundles (M_FILE / M_DATA / M_GOT)
//   - Receive inbound bundles
//   - Handle M_ERR and M_BSY
//
// BinkP framing (2-byte big-endian header):
//   bit 15  = 1 → command frame; bits 14-0 = data length of command
//   bit 15  = 0 → data frame;    bits 14-0 = data length
//
// Command byte occupies the first byte of the data portion of a command frame.
package fido

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BinkP command bytes.
const (
	bpM_NUL  byte = 0
	bpM_ADR  byte = 1
	bpM_PWD  byte = 2
	bpM_FILE byte = 3
	bpM_OK   byte = 4
	bpM_EOB  byte = 5
	bpM_GOT  byte = 6
	bpM_ERR  byte = 7
	bpM_BSY  byte = 8
	bpM_GET  byte = 9
	bpM_SKIP byte = 10
)

// PollResult describes the outcome of a BinkP poll.
type PollResult struct {
	Sent     []string // basenames of files sent
	Received []string // basenames of files received
	Error    error
}

// Poll dials the uplink, exchanges M_NUL/M_ADR/M_PWD, sends all files
// in outboundDir, receives any inbound files into inboundDir, then hangs up.
func Poll(nd *NetworkDef, outboundFiles []string, inboundDir string) *PollResult {
	res := &PollResult{}

	host := nd.Uplink
	if host == "" {
		res.Error = fmt.Errorf("no uplink configured for network %s", nd.Name)
		return res
	}

	// Strip point from uplink address for lookup — we just need host:port.
	addr, _ := ParseAddr(host)
	port := nd.Port()
	target := net.JoinHostPort(addrToIP(addr, host), strconv.Itoa(port))

	conn, err := net.DialTimeout("tcp", target, 30*time.Second)
	if err != nil {
		res.Error = fmt.Errorf("binkp dial %s: %w", target, err)
		return res
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Minute))

	bp := &binkpConn{conn: conn, nd: nd}

	// ── Handshake ────────────────────────────────────────────────────────────
	if err := bp.sendCmd(bpM_NUL, "SYS VirtBBS"); err != nil {
		res.Error = err; return res
	}
	if err := bp.sendCmd(bpM_NUL, "ZYZ "+nd.Address); err != nil {
		res.Error = err; return res
	}
	if err := bp.sendCmd(bpM_ADR, nd.Address); err != nil {
		res.Error = err; return res
	}

	// Wait for remote M_ADR before sending password.
	if err := bp.waitForADR(); err != nil {
		res.Error = err; return res
	}

	if err := bp.sendCmd(bpM_PWD, nd.Password); err != nil {
		res.Error = err; return res
	}

	// Wait for M_OK or M_ERR.
	if err := bp.waitForAuth(); err != nil {
		res.Error = err; return res
	}

	// ── Send outbound files ───────────────────────────────────────────────────
	for _, fpath := range outboundFiles {
		if err := bp.sendFile(fpath); err != nil {
			res.Error = err; return res
		}
		res.Sent = append(res.Sent, filepath.Base(fpath))
	}

	// Signal end-of-batch.
	if err := bp.sendCmd(bpM_EOB, ""); err != nil {
		res.Error = err; return res
	}

	// ── Receive inbound files until remote EOB ────────────────────────────────
	received, err := bp.receiveUntilEOB(inboundDir)
	if err != nil {
		res.Error = err; return res
	}
	res.Received = received

	// Final EOB / BYE exchange.
	_ = bp.sendCmd(bpM_EOB, "")
	return res
}

// ─── Internal BinkP connection ─────────────────────────────────────────────────

type binkpConn struct {
	conn net.Conn
	nd   *NetworkDef
}

// sendCmd sends a command frame: header (bit15=1, len=1+len(arg)) + cmd byte + arg bytes.
func (b *binkpConn) sendCmd(cmd byte, arg string) error {
	data := append([]byte{cmd}, []byte(arg)...)
	hdr := uint16(0x8000) | uint16(len(data))
	if err := binary.Write(b.conn, binary.BigEndian, hdr); err != nil {
		return err
	}
	_, err := b.conn.Write(data)
	return err
}

// sendData sends a data frame.
func (b *binkpConn) sendData(data []byte) error {
	hdr := uint16(len(data))
	if err := binary.Write(b.conn, binary.BigEndian, hdr); err != nil {
		return err
	}
	_, err := b.conn.Write(data)
	return err
}

// recvFrame reads one BinkP frame.  Returns (isCmd, cmdByte, payload, err).
func (b *binkpConn) recvFrame() (isCmd bool, cmd byte, payload []byte, err error) {
	var hdr uint16
	if err = binary.Read(b.conn, binary.BigEndian, &hdr); err != nil {
		return
	}
	isCmd = hdr&0x8000 != 0
	length := int(hdr & 0x7FFF)
	payload = make([]byte, length)
	if _, err = io.ReadFull(b.conn, payload); err != nil {
		return
	}
	if isCmd && len(payload) > 0 {
		cmd = payload[0]
		payload = payload[1:]
	}
	return
}

// waitForADR reads frames until M_ADR is received.
func (b *binkpConn) waitForADR() error {
	for {
		isCmd, cmd, _, err := b.recvFrame()
		if err != nil {
			return err
		}
		if isCmd && cmd == bpM_ADR {
			return nil
		}
		if isCmd && cmd == bpM_ERR {
			return fmt.Errorf("remote M_ERR during ADR")
		}
		if isCmd && cmd == bpM_BSY {
			return fmt.Errorf("remote busy (M_BSY)")
		}
	}
}

// waitForAuth reads frames until M_OK or M_ERR.
func (b *binkpConn) waitForAuth() error {
	for {
		isCmd, cmd, payload, err := b.recvFrame()
		if err != nil {
			return err
		}
		if isCmd {
			switch cmd {
			case bpM_OK:
				return nil
			case bpM_ERR:
				return fmt.Errorf("authentication failed: %s", string(payload))
			case bpM_BSY:
				return fmt.Errorf("remote busy: %s", string(payload))
			}
		}
	}
}

// sendFile sends one file using M_FILE + M_DATA frames then waits for M_GOT.
func (b *binkpConn) sendFile(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	size := fi.Size()
	mtime := fi.ModTime().Unix()

	// M_FILE <name> <size> <mtime> <offset>
	fileArg := fmt.Sprintf("%s %d %d 0", filepath.Base(path), size, mtime)
	if err := b.sendCmd(bpM_FILE, fileArg); err != nil {
		return err
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 8192)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			if serr := b.sendData(buf[:n]); serr != nil {
				return serr
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	// Wait for M_GOT acknowledgement for this file.
	return b.waitForGOT(filepath.Base(path), size)
}

// waitForGOT reads frames until M_GOT for the named file arrives.
func (b *binkpConn) waitForGOT(name string, size int64) error {
	for {
		isCmd, cmd, payload, err := b.recvFrame()
		if err != nil {
			return err
		}
		if !isCmd {
			continue // data frames during GOT wait are silently discarded
		}
		switch cmd {
		case bpM_GOT:
			// payload: "<name> <size> <mtime>"
			parts := strings.SplitN(string(payload), " ", 2)
			if parts[0] == name {
				return nil
			}
		case bpM_SKIP:
			return fmt.Errorf("remote skipped %s", name)
		case bpM_ERR:
			return fmt.Errorf("remote error: %s", string(payload))
		case bpM_GET:
			// Restart file from offset — not implemented, skip.
		}
	}
}

// receiveUntilEOB reads files until a remote M_EOB frame or error.
// Files are written to destDir.
func (b *binkpConn) receiveUntilEOB(destDir string) ([]string, error) {
	var received []string
	var currentFile *os.File
	var currentName string
	var currentSize, received_bytes int64

	for {
		isCmd, cmd, payload, err := b.recvFrame()
		if err != nil {
			if currentFile != nil {
				currentFile.Close()
			}
			return received, err
		}

		if isCmd {
			switch cmd {
			case bpM_EOB:
				if currentFile != nil {
					currentFile.Close()
					currentFile = nil
				}
				return received, nil

			case bpM_FILE:
				// Close previous file if open.
				if currentFile != nil {
					currentFile.Close()
					currentFile = nil
				}
				// Parse: "<name> <size> <mtime> [offset]"
				parts := strings.Fields(string(payload))
				if len(parts) < 3 {
					continue
				}
				currentName = parts[0]
				fmt.Sscanf(parts[1], "%d", &currentSize)
				received_bytes = 0

				destPath := filepath.Join(destDir, currentName)
				currentFile, err = os.Create(destPath)
				if err != nil {
					return received, fmt.Errorf("create inbound %s: %w", destPath, err)
				}

			case bpM_ERR:
				return received, fmt.Errorf("remote M_ERR: %s", string(payload))

			case bpM_GOT:
				// Sent by remote for our files — already handled in sendFile.

			case bpM_NUL, bpM_ADR, bpM_OK:
				// Informational during transfer — ignore.
			}
		} else {
			// Data frame — write to current inbound file.
			if currentFile != nil {
				if _, err := currentFile.Write(payload); err != nil {
					currentFile.Close()
					return received, err
				}
				received_bytes += int64(len(payload))
				if received_bytes >= currentSize {
					currentFile.Close()
					currentFile = nil
					received = append(received, currentName)
					// Send M_GOT acknowledgement.
					gotArg := fmt.Sprintf("%s %d 0", currentName, currentSize)
					_ = b.sendCmd(bpM_GOT, gotArg)
					currentName = ""
					currentSize = 0
					received_bytes = 0
				}
			}
		}
	}
}

// addrToIP returns a host string to dial.
// For numeric-looking addresses it uses the uplink string itself,
// otherwise it returns the raw uplink field as a hostname.
// In production this would do a NODELIST lookup — for now we just
// return the uplink string stripped of the FidoNet address portion.
func addrToIP(a Addr, raw string) string {
	// If the raw uplink looks like "1:234/567@fidonet.org", extract the hostname.
	if idx := strings.Index(raw, "@"); idx >= 0 {
		return raw[idx+1:]
	}
	// Otherwise treat as DNS name / IP (common for configured uplinks).
	return raw
}
