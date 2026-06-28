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
//   v0.0.3  2026-06-24  Phase 9: FidoNet toss — import .PKT into message store
//   v0.1.0  2026-06-25  Use Parse() to preserve MSGID/SEEN-BY/PATH/origin metadata
//                        instead of discarding it; dedupe inbound messages by MSGID
//   v0.1.1  2026-06-25  Auto-respond to inbound PING netmail with a PONG reply
//   v0.2.0  2026-06-25  Route netmail addressed to "AreaFix" to the AreaFix
//                        responder instead of normal storage
//   v0.3.0  2026-06-25  TossDir/TossFile now take a *NetworkDef instead of
//                        *Config, so any configured network can be tossed
//                        (not just the primary) — needed by the scheduler
//   v0.4.0  2026-06-25  Add TossAll, looping every enabled network (mirrors
//                        ScanAll), used by the sysop/API/CLI toss commands so
//                        AreaFix/FileFix/PING/TRACE work for every network,
//                        not just primary
// ============================================================================

package fido

// Toss processes all .PKT files in a directory, importing messages into the
// VirtBBS message store according to the area→conference map in Config.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// PrimaryNetworkName is the network name AllNetworks() always assigns to
// the primary (top-level [fido]) network. Used by call sites that are not
// (yet) network-aware, such as the AreaFix downlink admin menu.
const PrimaryNetworkName = "FidoNet"

// TossResult summarises the outcome of a toss run.
type TossResult struct {
	Packets          int // .PKT files processed
	Imported         int // messages inserted
	Skipped          int // messages ignored (duplicate, etc.)
	Orphaned         int // messages held for sysop review
	NetmailImported  int
	EchomailImported int
	NetmailSkipped   int
	EchomailSkipped  int
	NetmailHeld      int
	EchomailHeld     int
	OrphanNotes      []OrphanNote
	Errors           []string
	TICProcessed     int
}

// TossAll tosses every enabled network's inbound directory in turn,
// aggregating the results. Disabled networks are skipped. Used wherever
// "toss inbound mail" should mean *all* configured networks, not just the
// primary one (sysop [T]oss menu, fido.toss API, -fido-toss CLI flag).
func TossAll(cfg *Config, store *messages.Store, confStore *conferences.Store, sysopName string, fileArea FileArea, filesRoot string) *TossResult {
	total := &TossResult{}
	for _, nd := range cfg.AllNetworks() {
		if !nd.Enabled {
			continue
		}
		r, err := TossDir(&nd, store, confStore, sysopName, fileArea, filesRoot)
		if err != nil {
			total.Errors = append(total.Errors, fmt.Sprintf("[%s] %v", nd.Name, err))
			continue
		}
		total.Packets += r.Packets
		total.Imported += r.Imported
		total.Skipped += r.Skipped
		total.Orphaned += r.Orphaned
		total.TICProcessed += r.TICProcessed
		total.OrphanNotes = append(total.OrphanNotes, r.OrphanNotes...)
		for _, e := range r.Errors {
			total.Errors = append(total.Errors, fmt.Sprintf("[%s] %s", nd.Name, e))
		}
	}
	return total
}

// TossDir reads every .PKT file in nd.InboundDir, imports all recognised
// echomail messages, and moves processed packets to <inbound>/.tossed/.
// confStore may be nil (AreaFix's %LIST falls back to nd.Areas and area
// validation is skipped for tag existence checks).
func TossDir(nd *NetworkDef, store *messages.Store, confStore *conferences.Store, sysopName string, fileArea FileArea, filesRoot string) (*TossResult, error) {
	if !nd.Enabled {
		return nil, fmt.Errorf("network %s is disabled", nd.Name)
	}
	if err := os.MkdirAll(nd.InboundDir, 0755); err != nil {
		return nil, err
	}
	tossed := filepath.Join(nd.InboundDir, ".tossed")
	if err := os.MkdirAll(tossed, 0755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(nd.EffectiveHoldingDir(), 0755); err != nil {
		return nil, err
	}

	result := &TossResult{}

	entries, err := os.ReadDir(nd.InboundDir)
	if err != nil {
		return nil, fmt.Errorf("read inbound dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".pkt") {
			continue
		}

		pktPath := filepath.Join(nd.InboundDir, e.Name())
		imp, skip, orphans, ni, ei, ns, es, nh, eh, notes, errs := tossFile(nd, store, confStore, filesRoot, pktPath)
		result.Packets++
		result.Imported += imp
		result.Skipped += skip
		result.Orphaned += orphans
		result.NetmailImported += ni
		result.EchomailImported += ei
		result.NetmailSkipped += ns
		result.EchomailSkipped += es
		result.NetmailHeld += nh
		result.EchomailHeld += eh
		result.OrphanNotes = append(result.OrphanNotes, notes...)
		result.Errors = append(result.Errors, errs...)

		// Move processed packet to .tossed/.
		dest := filepath.Join(tossed, e.Name())
		_ = os.Rename(pktPath, dest)
	}

	if len(result.OrphanNotes) > 0 {
		if err := NotifySysopOrphans(store, sysopName, nd.Name, result.OrphanNotes); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("sysop notify: %v", err))
		}
	}
	RecordToss(nd.Name, result)
	if fileArea != nil {
		ticRes := ProcessInboundTICs(nd, store.DB(), fileArea)
		result.TICProcessed = ticRes.Processed
		for _, e := range ticRes.Errors {
			result.Errors = append(result.Errors, "tic: "+e)
		}
		for _, e := range ProcessPendingNodelistEchoesForNetwork(store.DB(), fileArea, nd.Name) {
			result.Errors = append(result.Errors, "nodelist echo: "+e)
		}
	}
	return result, nil
}

// TossFile processes a single .PKT file, importing its messages.
func TossFile(nd *NetworkDef, store *messages.Store, confStore *conferences.Store, sysopName, filesRoot, pktPath string) (imported, skipped, orphaned int, notes []OrphanNote, errs []string) {
	imp, sk, orph, _, _, _, _, _, _, nts, es := tossFile(nd, store, confStore, filesRoot, pktPath)
	return imp, sk, orph, nts, es
}

func tossFile(nd *NetworkDef, store *messages.Store, confStore *conferences.Store, filesRoot, pktPath string) (
	imported, skipped, orphaned, netImported, echoImported, netSkipped, echoSkipped, netHeld, echoHeld int,
	notes []OrphanNote, errs []string,
) {
	f, err := os.Open(pktPath)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s: %v", pktPath, err))
		return
	}
	defer f.Close()

	msgs, err := ReadPacket(f)
	if err != nil {
		errs = append(errs, fmt.Sprintf("%s: parse error: %v", pktPath, err))
		return
	}

	for _, pm := range msgs {
		pb := pm.Parse()
		area := pb.AreaTag
		isNetmail := area == ""

		if isNetmail {
			// Auto-respond to PING test messages with a PONG reply. IsPing
			// only matches "PING" exactly, so a PONG reaching us here never
			// triggers another reply — no loop-guard needed beyond that.
			if IsPing(pm.Subject) {
				if err := AutoRespondPing(nd, pm); err != nil {
					errs = append(errs, fmt.Sprintf("ping auto-reply: %v", err))
				}
			}

			// Same convention for TRACE — IsTrace only matches "TRACE"
			// exactly, so a TRACE REPLY reaching us here never triggers
			// another reply.
			if IsTrace(pm.Subject) {
				if err := AutoRespondTrace(nd, pm); err != nil {
					errs = append(errs, fmt.Sprintf("trace auto-reply: %v", err))
				}
			}

			// AreaFix/FileFix requests are handled by their responders and
			// still stored as ordinary netmail below, so the sysop can
			// audit what downlinks have requested.
			if IsAreaFixRequest(pm.ToName) {
				if err := ProcessAreaFixRequest(nd, store, confStore, nd.Name, "", pm); err != nil {
					errs = append(errs, fmt.Sprintf("areafix: %v", err))
				}
			}
			if IsFileFixRequest(pm.ToName) {
				if err := ProcessFileFixRequest(nd, store.DB(), filesRoot, pm); err != nil {
					errs = append(errs, fmt.Sprintf("filefix: %v", err))
				}
			}

			// VirtNet: a delegated sub-hub announcing a node it registered
			// under its own net. Only meaningful at the real central hub
			// (Uplink==""); a sub-hub receiving this would have nowhere
			// further to apply it, so it's a no-op there.
			if IsNodeAnnounceRequest(pm.Subject) && nd.IsHub() {
				if err := ProcessNodeAnnounce(nd, store.DB(), confStore, store, pm); err != nil {
					errs = append(errs, fmt.Sprintf("node announce: %v", err))
				}
			}

			// Netmail not addressed to this node — hold for sysop review
			// instead of importing to conference 0.
			if !IsAreaFixRequest(pm.ToName) && !IsFileFixRequest(pm.ToName) &&
				!IsPing(pm.Subject) && !IsTrace(pm.Subject) &&
				!(IsNodeAnnounceRequest(pm.Subject) && nd.IsHub()) &&
				!nd.MatchesOurAddr(pm.DestAddr) {
				if note, err := HoldOrphanMessage(nd, pm, "NOT_FOR_US"); err != nil {
					errs = append(errs, fmt.Sprintf("hold netmail: %v", err))
					skipped++
					netSkipped++
				} else {
					orphaned++
					netHeld++
					notes = append(notes, note)
				}
				continue
			}
		}

		var confID int
		if isNetmail {
			confID = 0
		} else {
			confID = nd.ConferenceForArea(area)
			if confID < 0 {
				if note, err := HoldOrphanMessage(nd, pm, "UNKNOWN_AREA"); err != nil {
					errs = append(errs, fmt.Sprintf("hold echomail: %v", err))
					skipped++
					echoSkipped++
				} else {
					orphaned++
					echoHeld++
					notes = append(notes, note)
				}
				continue
			}

			// VirtNet: this network's own generated nodelist, distributed
			// as ordinary echomail (see EnsureEchoConference/scan.go's
			// existing downlink fan-out — no new transport code needed for
			// distribution). Queue it for ProcessPendingNodelistEchoes
			// rather than processing inline, since applying it needs
			// internal/files, which internal/fido cannot import.
			if strings.EqualFold(area, nd.EffectiveNodelistEchoTag()) {
				if err := QueueNodelistEcho(store.DB(), nd.Name, pm.Subject, pb.Text); err != nil {
					errs = append(errs, fmt.Sprintf("queue nodelist echo: %v", err))
				}
			}
		}

		// Idempotency: skip if this exact message (by MSGID) was already
		// imported into this conference. Guards against re-processing the
		// same .PKT twice, e.g. a crash between import and moving the file
		// to .tossed/.
		if pb.MSGID != "" {
			exists, err := store.HasFidoMsgID(confID, pb.MSGID)
			if err != nil {
				errs = append(errs, fmt.Sprintf("dedupe check: %v", err))
			} else if exists {
				skipped++
				if isNetmail {
					netSkipped++
				} else {
					echoSkipped++
				}
				continue
			}
		}

		// Parse date from FTS dateTime string "dd Mon yy  hh:mm:ss"
		posted := parseFidoDate(pm.DateTime)

		m := &messages.Message{
			ConferenceID: confID,
			FromName:     pm.FromName,
			ToName:       pm.ToName,
			Subject:      pm.Subject,
			DatePosted:   posted,
			Status:       "A",
			Echo:         area != "",
			Body:         pb.Text,
			FidoMsgID:    pb.MSGID,
			FidoReply:    pb.REPLY,
			FidoKludges:  pb.Kludges,
			FidoSeenBy:   strings.Join(pb.SeenBy, " "),
			FidoPath:     strings.Join(pb.Path, " "),
			FidoOrigin:   pm.OrigAddr.String(),
		}
		if err := store.Post(m); err != nil {
			errs = append(errs, fmt.Sprintf("insert: %v", err))
			if note, holdErr := HoldOrphanMessage(nd, pm, "INSERT_FAILED"); holdErr != nil {
				errs = append(errs, fmt.Sprintf("hold after insert fail: %v", holdErr))
				skipped++
				if isNetmail {
					netSkipped++
				} else {
					echoSkipped++
				}
			} else {
				orphaned++
				if isNetmail {
					netHeld++
				} else {
					echoHeld++
				}
				notes = append(notes, note)
			}
			continue
		}
		imported++
		if isNetmail {
			netImported++
		} else {
			echoImported++
		}
	}
	return
}

// parseFidoDate parses an FTS-0001 date string.
// Format: "dd Mon yy  hh:mm:ss" (e.g. "25 Jun 24  14:30:00")
func parseFidoDate(s string) time.Time {
	// Try multiple common FidoNet date formats.
	formats := []string{
		"02 Jan 06  15:04:05",
		"02 Jan 06 15:04:05",
		"_2 Jan 06  15:04:05",
		"Mon Jan  2 15:04:05 2006",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Now()
}
