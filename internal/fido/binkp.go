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
//   v0.3.0  2026-06-25  Add PollAndToss, combining a poll with an automatic toss of
//                        whatever was received, for the scheduler and sysop/API poll
//                        commands to share
//   v0.4.0  2026-06-25  Add ServeBinkP — a BinkP server accepting inbound connections
//                        from configured uplinks and downlinks, so other systems can
//                        poll THIS BBS instead of only the reverse
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

	"database/sql"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
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
// db is used to resolve FidoNet-address uplinks via the imported nodelist.
func Poll(nd *NetworkDef, outboundFiles []string, inboundDir string, db *sql.DB) *PollResult {
	res := &PollResult{}

	if nd.Uplink == "" {
		res.Error = fmt.Errorf("no uplink configured for network %s", nd.Name)
		return res
	}

	dialHost, dialPort, err := ResolveBinkpDialTarget(nd.Name, nd.Uplink, nd.Port(), db)
	if err != nil {
		res.Error = err
		return res
	}
	if dialPort == 0 {
		dialPort = nd.Port()
	}
	target := net.JoinHostPort(dialHost, strconv.Itoa(dialPort))

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
	if err := bp.sendCmd(bpM_ADR, strings.Join(nd.AllAddrsString(), " ")); err != nil {
		res.Error = err; return res
	}

	// Wait for remote M_ADR before sending password.
	if err := bp.waitForADR(); err != nil {
		res.Error = fmt.Errorf("handshake ADR (%s): %w", target, err); return res
	}

	pwd := nd.Password
	if pwd == "" {
		pwd = "-"
	}
	if err := bp.sendCmd(bpM_PWD, pwd); err != nil {
		res.Error = fmt.Errorf("handshake PWD (%s): %w", target, err); return res
	}

	// Wait for M_OK or M_ERR.
	if err := bp.waitForAuth(); err != nil {
		res.Error = fmt.Errorf("handshake auth (%s): %w", target, err); return res
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
		res.Error = fmt.Errorf("receive (%s): %w", target, err); return res
	}
	res.Received = received

	// Final EOB / BYE exchange.
	_ = bp.sendCmd(bpM_EOB, "")
	return res
}

// PollAndTossResult combines a BinkP poll outcome with the toss that
// automatically follows it.
type PollAndTossResult struct {
	Poll *PollResult
	Toss *TossResult // nil if the poll itself failed, so nothing was tossed
}

// PollAndToss gathers nd's outbound files, polls its uplink, deletes any
// successfully-sent files, and then — regardless of whether anything new
// was received this time — tosses nd's inbound directory, so any mail
// left over from a previous partial failure is also picked up.
//
// This is the single entry point shared by the sysop "[P]oll uplink" menu,
// the "fido.poll" management API, and the automatic scheduler, so all three
// behave identically.
func PollAndToss(nd *NetworkDef, store *messages.Store, confStore *conferences.Store, sysopName string) *PollAndTossResult {
	if store != nil {
		if qr := ScanNetmailQueue(nd, store.DB()); qr != nil {
			for _, e := range qr.Errors {
				LogBinkp(fmt.Sprintf("netmail queue [%s]: %s", nd.Name, e))
			}
			if qr.Exported > 0 {
				LogBinkp(fmt.Sprintf("netmail queue [%s]: exported %d message(s) to outbound", nd.Name, qr.Exported))
			}
		}
	}

	uplink := nd.UplinkAddr()
	outFiles := binkpOutboundFilesFor(nd, nil, uplink)

	pollResult := Poll(nd, outFiles, nd.InboundDir, store.DB())
	result := &PollAndTossResult{Poll: pollResult}
	uplinkKey := nd.Uplink
	if pollResult.Error != nil {
		logPollResult(nd.Name, "client", len(pollResult.Sent), len(pollResult.Received), pollResult.Error)
		RecordClientPoll(nd.Name, uplinkKey, false, len(pollResult.Sent), len(pollResult.Received))
		return result
	}

	sentBase := make(map[string]bool, len(pollResult.Sent))
	for _, f := range pollResult.Sent {
		sentBase[f] = true
	}
	for _, full := range outFiles {
		if sentBase[filepath.Base(full)] {
			_ = os.Remove(full)
		}
	}

	tossResult, err := TossDir(nd, store, confStore, sysopName)
	if err != nil {
		tossResult = &TossResult{Errors: []string{err.Error()}}
	}
	result.Toss = tossResult
	logPollResult(nd.Name, "client", len(pollResult.Sent), len(pollResult.Received), nil)
	logTossResult(nd.Name, "client", result.Toss)
	RecordClientPoll(nd.Name, uplinkKey, true, len(pollResult.Sent), len(pollResult.Received))
	return result
}

// ─── Server (accepting inbound polls) ──────────────────────────────────────────

// ServeBinkP listens on every distinct binkp_port configured among enabled
// networks (a single port shared by several networks is handled — each
// inbound connection's identity, from its M_ADR, is matched against every
// enabled network's uplink and downlink addresses to find which one it
// belongs to). The caller is authenticated by password: the downlink's own
// configured password if it matched a Downlink, or the network's session
// Password if it matched the network's own uplink address.
//
// After exchanging files, any received inbound is tossed immediately
// (matching the "every poll completes with a toss" behaviour of the
// outbound side — see PollAndToss).
//
// Returns a stop function that closes all listeners. Logs session activity
// and errors with the standard logger; never returns an error itself once
// listening has started (per-connection failures are logged, not fatal).
func ServeBinkP(cfg *Config, store *messages.Store, confStore *conferences.Store, sysopName string) (stop func(), err error) {
	portCandidates := map[int][]NetworkDef{}
	for _, nd := range cfg.AllNetworks() {
		if !nd.Enabled {
			continue
		}
		portCandidates[nd.Port()] = append(portCandidates[nd.Port()], nd)
	}
	if len(portCandidates) == 0 {
		return func() {}, nil
	}

	var listeners []net.Listener
	for port, candidates := range portCandidates {
		addr := fmt.Sprintf(":%d", port)
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			for _, l := range listeners {
				l.Close()
			}
			return nil, fmt.Errorf("binkp listen %s: %w", addr, lerr)
		}
		listeners = append(listeners, ln)
		LogBinkp(fmt.Sprintf("BinkP listening on %s (%d network(s))", addr, len(candidates)))
		go binkpAcceptLoop(ln, candidates, store, confStore, sysopName)
	}

	return func() {
		for _, l := range listeners {
			l.Close()
		}
	}, nil
}

func binkpAcceptLoop(ln net.Listener, candidates []NetworkDef, store *messages.Store, confStore *conferences.Store, sysopName string) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		go binkpHandleIncoming(conn, candidates, store, confStore, sysopName)
	}
}

// binkpHandleIncoming answers one inbound BinkP connection: handshake,
// identify and authenticate the caller, receive their files, send back
// whatever is queued for them, then toss what was received.
func binkpHandleIncoming(conn net.Conn, candidates []NetworkDef, store *messages.Store, confStore *conferences.Store, sysopName string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Minute))
	bp := &binkpConn{conn: conn}

	_ = bp.sendCmd(bpM_NUL, "SYS VirtBBS")
	if len(candidates) > 0 {
		_ = bp.sendCmd(bpM_NUL, "ZYZ "+candidates[0].Address)
		_ = bp.sendCmd(bpM_ADR, strings.Join(candidates[0].AllAddrsString(), " "))
	}

	peerAddrs, err := binkpWaitForADRAddrs(bp)
	if err != nil {
		LogBinkp(fmt.Sprintf("binkp server: handshake error from %s: %v", conn.RemoteAddr(), err))
		RecordSessionError("")
		return
	}

	nd, dl, isUplink := binkpMatchPeer(candidates, peerAddrs)
	if nd == nil {
		_ = bp.sendCmd(bpM_ERR, "unknown system")
		LogBinkp(fmt.Sprintf("binkp server: rejected unknown peer %v from %s", peerAddrs, conn.RemoteAddr()))
		RecordSessionError("")
		return
	}

	wantPassword := nd.Password
	if !isUplink && dl != nil {
		wantPassword = dl.Password
	}
	linkType := "downlink"
	peerKey := ""
	if isUplink {
		linkType = "uplink"
		peerKey = nd.Uplink
	} else if dl != nil {
		if dl.Address != "" {
			peerKey = dl.Address
		} else {
			peerKey = dl.Name
		}
	}
	if peerKey == "" && len(peerAddrs) > 0 {
		peerKey = peerAddrs[0]
	}

	if wantPassword != "" {
		gotPwd, err := binkpWaitForPWD(bp)
		if err != nil {
			LogBinkp(fmt.Sprintf("binkp server [%s]: password handshake error: %v", nd.Name, err))
			RecordSessionError(nd.Name)
			RecordServerSession(nd.Name, linkType, peerKey, false, 0, 0)
			return
		}
		if gotPwd != wantPassword {
			_ = bp.sendCmd(bpM_ERR, "bad password")
			LogBinkp(fmt.Sprintf("binkp server [%s]: bad password from %v", nd.Name, peerAddrs))
			RecordSessionError(nd.Name)
			RecordServerSession(nd.Name, linkType, peerKey, false, 0, 0)
			return
		}
	}
	if err := bp.sendCmd(bpM_OK, ""); err != nil {
		return
	}

	if err := os.MkdirAll(nd.InboundDir, 0755); err != nil {
		LogBinkp(fmt.Sprintf("binkp server [%s]: %v", nd.Name, err))
		RecordSessionError(nd.Name)
		RecordServerSession(nd.Name, linkType, peerKey, false, 0, 0)
		return
	}
	received, err := bp.receiveUntilEOB(nd.InboundDir)
	if err != nil {
		LogBinkp(fmt.Sprintf("binkp server [%s]: receive error: %v", nd.Name, err))
		RecordSessionError(nd.Name)
		RecordServerSession(nd.Name, linkType, peerKey, false, 0, len(received))
		return
	}

	peerAddr, _ := ParseAddr(peerAddrs[0])
	outFiles := binkpOutboundFilesFor(nd, dl, peerAddr)
	var sent []string
	for _, f := range outFiles {
		if err := bp.sendFile(f); err != nil {
			LogBinkp(fmt.Sprintf("binkp server [%s]: send error: %v", nd.Name, err))
			break
		}
		sent = append(sent, f)
	}
	_ = bp.sendCmd(bpM_EOB, "")
	for _, f := range sent {
		_ = os.Remove(f)
	}

	who := "uplink"
	if !isUplink {
		who = "downlink " + dl.Name
	}
	LogBinkp(fmt.Sprintf("binkp server [%s]: session with %s (%v) complete — received %d, sent %d",
		nd.Name, who, peerAddrs, len(received), len(sent)))
	RecordServerSession(nd.Name, linkType, peerKey, true, len(sent), len(received))

	if len(received) > 0 {
		if tr, err := TossDir(nd, store, confStore, sysopName); err != nil {
			LogBinkp(fmt.Sprintf("binkp server [%s]: auto-toss error: %v", nd.Name, err))
		} else {
			logTossResult(nd.Name, "server", tr)
		}
	}
}

// binkpWaitForADRAddrs reads frames until the caller's M_ADR arrives,
// returning its space-separated address list.
func binkpWaitForADRAddrs(b *binkpConn) ([]string, error) {
	for {
		isCmd, cmd, payload, err := b.recvFrame()
		if err != nil {
			return nil, err
		}
		if !isCmd {
			continue
		}
		switch cmd {
		case bpM_ADR:
			return strings.Fields(string(payload)), nil
		case bpM_ERR:
			return nil, fmt.Errorf("remote M_ERR during handshake: %s", string(payload))
		case bpM_BSY:
			return nil, fmt.Errorf("remote busy")
		}
	}
}

// binkpWaitForPWD reads frames until the caller's M_PWD arrives.
func binkpWaitForPWD(b *binkpConn) (string, error) {
	for {
		isCmd, cmd, payload, err := b.recvFrame()
		if err != nil {
			return "", err
		}
		if !isCmd {
			continue
		}
		switch cmd {
		case bpM_PWD:
			return string(payload), nil
		case bpM_ERR:
			return "", fmt.Errorf("remote M_ERR: %s", string(payload))
		}
	}
}

// binkpMatchPeer finds which candidate network (and, if applicable, which
// configured Downlink) a caller's announced addresses belong to: either
// the network's own uplink address (isUplink=true), or one of its
// configured downlinks (isUplink=false, dl set).
func binkpMatchPeer(candidates []NetworkDef, peerAddrs []string) (nd *NetworkDef, dl *Downlink, isUplink bool) {
	for i := range candidates {
		c := candidates[i]
		uplink := c.UplinkAddr()
		for _, pa := range peerAddrs {
			a, err := ParseAddr(pa)
			if err != nil {
				continue
			}
			if uplink != (Addr{}) && a.Zone == uplink.Zone && a.Net == uplink.Net && a.Node == uplink.Node {
				return &c, nil, true
			}
			if found := c.DownlinkByAddr(a); found != nil {
				return &c, found, false
			}
		}
	}
	return nil, nil, false
}

// binkpOutboundFilesFor returns the paths of files queued for a specific
// peer: if dl is set (the peer is a known downlink), only files whose name
// was tagged with that downlink's address by the AreaFix scan-time fan-out
// (see scan.go's sanitizeAddrForFilename); otherwise (the peer is our
// uplink) every file NOT specifically tagged for one of our own downlinks.
// Either way, any crash-routed netmail queued in that peer's OutboundDir
// subdirectory is included too.
func binkpOutboundFilesFor(nd *NetworkDef, dl *Downlink, peerAddr Addr) []string {
	var out []string
	entries, _ := os.ReadDir(nd.OutboundDir)

	if dl != nil {
		tag := sanitizeAddrForFilename(peerAddr)
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pkt") {
				continue
			}
			if strings.Contains(e.Name(), tag) {
				out = append(out, filepath.Join(nd.OutboundDir, e.Name()))
			}
		}
	} else {
		for _, e := range entries {
			if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".pkt") {
				continue
			}
			taggedForADownlink := false
			for _, other := range nd.Downlinks {
				a, err := ParseAddr(other.Address)
				if err != nil {
					continue
				}
				if strings.Contains(e.Name(), sanitizeAddrForFilename(a)) {
					taggedForADownlink = true
					break
				}
			}
			if !taggedForADownlink {
				out = append(out, filepath.Join(nd.OutboundDir, e.Name()))
			}
		}
	}

	sub := fmt.Sprintf("%04X%04X.OUT", peerAddr.Zone*0x100+peerAddr.Net, peerAddr.Node)
	crashDir := filepath.Join(nd.OutboundDir, sub)
	if crashEntries, err := os.ReadDir(crashDir); err == nil {
		for _, e := range crashEntries {
			if !e.IsDir() {
				out = append(out, filepath.Join(crashDir, e.Name()))
			}
		}
	}

	// VirtNet: for networks this BBS hosts (no uplink), unconditionally
	// offer the latest generated nodelist alongside whatever's tagged for
	// this peer — every member gets the current nodelist on every poll,
	// not just via the echomail distribution path (nodelistecho.go).
	if nd.IsHub() {
		if latest := latestNodelistFile(nd.NodelistDir, "VirtNode.Z"); latest != "" {
			out = append(out, latest)
		}
		if latest := latestNodelistFile(nd.NodelistDir, "VirtNode.D"); latest != "" {
			out = append(out, latest)
		}
	}
	return out
}

// latestNodelistFile finds the most recently modified file in dir whose
// name starts with prefix (e.g. "VirtNode.Z" or "VirtNode.D").
func latestNodelistFile(dir, prefix string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestTime int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if fi.ModTime().UnixNano() > bestTime {
			bestTime = fi.ModTime().UnixNano()
			best = filepath.Join(dir, e.Name())
		}
	}
	return best
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
		return fmt.Errorf("M_FILE %s: %w", filepath.Base(path), err)
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
				return fmt.Errorf("M_DATA %s: %w", filepath.Base(path), serr)
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
	if err := b.waitForGOT(filepath.Base(path), size); err != nil {
		return fmt.Errorf("M_GOT %s: %w", filepath.Base(path), err)
	}
	return nil
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
				return received, err
			}
			// Some BinkP hosts (e.g. Synchronet/sbbs, binkd) close the TCP
			// session after the batch instead of sending a final M_EOB.
			if err == io.EOF {
				return received, nil
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
