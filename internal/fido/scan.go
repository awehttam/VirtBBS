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
//   v0.0.3  2026-06-24  Phase 9: FidoNet scan — export messages to .PKT
//   v0.0.6  2026-06-24  Multi-uplink bundling; per-conference uplink_addr override;
//                        network-aware scanning via conferences.Store
//   v0.1.0  2026-06-25  TZUTC/MSGID kludges, standard tear+Origin line (replacing
//                        the non-standard \x01ORIGIN kludge), configurable taglines,
//                        SEEN-BY/PATH construction (parsing+merging inbound values),
//                        and fix the resend-loop bug: messages are now marked
//                        exported only after a successful PKT write, and ListEcho
//                        only returns not-yet-exported messages.
//   v0.2.0  2026-06-25  Fan outgoing echomail out to AreaFix-subscribed downlinks,
//                        in addition to the conference's configured uplink
// ============================================================================

package fido

// Scan exports echo-flagged messages from the VirtBBS message store into
// outbound .PKT files, one PKT per unique uplink address.
//
// Each echomail conference can override the default uplink via its UplinkAddr
// field.  Messages destined for the same uplink are bundled into one PKT file.
// If no conference overrides apply, all messages go into a single PKT addressed
// to the default uplink.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/version"
)

// ScanResult summarises the outcome of a scan run.
type ScanResult struct {
	Scanned  int // messages exported
	PKTFiles int // distinct .pkt files written
	Errors   []string
}

// ScanAll exports all echo-flagged messages from every configured echomail
// conference into outbound .PKT files, one file per unique uplink address.
//
// It accepts an optional conferences.Store; when nil, falls back to the
// old cfg.Areas map (compatibility with pre-v0.0.6 setups). bbsName is used
// in the Origin line of each exported message.
func ScanAll(cfg *Config, store *messages.Store, confStore *conferences.Store, bbsName string) (*ScanResult, error) {
	if !cfg.Enabled {
		return nil, fmt.Errorf("FidoNet is disabled in config")
	}

	result := &ScanResult{}

	// Process every configured network (primary + additional).
	for _, nd := range cfg.AllNetworks() {
		if !nd.Enabled {
			continue
		}
		taglines := LoadTaglines(nd.TaglinesFile)
		areafixDB := OpenAreaFixDB(store.DB())
		if err := scanNetwork(cfg, &nd, store, confStore, bbsName, taglines, areafixDB, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("[%s] %v", nd.Name, err))
		}
	}
	return result, nil
}

// bucketEntry pairs a packet-ready message with the source DB message ID,
// so it can be marked exported only after the PKT containing it is
// successfully written (preserving at-least-once delivery on write failure).
type bucketEntry struct {
	pmsg *Message
	id   int64
}

// scanNetwork processes one network.
func scanNetwork(cfg *Config, nd *NetworkDef, store *messages.Store, confStore *conferences.Store,
	bbsName string, taglines []string, areafixDB *AreaFixDB, result *ScanResult) error {
	orig := nd.NodeAddr()
	defaultUplink := nd.UplinkAddr()

	if orig == (Addr{}) {
		return fmt.Errorf("invalid node address %q", nd.Address)
	}
	if defaultUplink == (Addr{}) {
		return fmt.Errorf("invalid uplink address %q", nd.Uplink)
	}

	if err := os.MkdirAll(nd.OutboundDir, 0755); err != nil {
		return err
	}

	// per-destination message bucket (uplink OR a subscribed downlink):
	// destAddr.String() → []bucketEntry
	buckets := map[string][]bucketEntry{}
	// destination address cache for writing PKTs
	destAddrs := map[string]Addr{}

	// ── Build buckets ─────────────────────────────────────────────────────────

	if confStore != nil {
		// Use conference store: iterate echomail conferences for this network.
		confs, err := confStore.ListEcho(nd.Name)
		if err != nil {
			return fmt.Errorf("listing echo confs: %w", err)
		}

		for _, conf := range confs {
			if conf.EchoTag == "" {
				continue
			}

			// Determine uplink for this conference.
			uplinkAddr := defaultUplink
			if conf.UplinkAddr != "" {
				if a, err := ParseAddr(conf.UplinkAddr); err == nil {
					uplinkAddr = a
				}
			}
			key := uplinkAddr.String()
			destAddrs[key] = uplinkAddr

			msgs, err := store.ListEcho(conf.ID, 500, 0)
			if err != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("conf %d (%s): %v", conf.ID, conf.Name, err))
				continue
			}
			for _, m := range msgs {
				appendEchoMessage(buckets, destAddrs, key, m, conf.EchoTag, orig, uplinkAddr,
					bbsName, taglines, nd, areafixDB, result)
			}
		}
	} else {
		// Fall back to cfg.Areas map (primary network only).
		for areaTag, confID := range nd.Areas {
			msgs, err := store.ListEcho(confID, 500, 0)
			if err != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("area %s conf %d: %v", areaTag, confID, err))
				continue
			}
			key := defaultUplink.String()
			destAddrs[key] = defaultUplink
			for _, m := range msgs {
				appendEchoMessage(buckets, destAddrs, key, m, areaTag, orig, defaultUplink,
					bbsName, taglines, nd, areafixDB, result)
			}
		}
	}

	// ── Write one PKT per destination bucket, then mark messages exported ────

	for key, entries := range buckets {
		if len(entries) == 0 {
			continue
		}
		destAddr := destAddrs[key]
		pktName := filepath.Join(nd.OutboundDir,
			fmt.Sprintf("%s_%s_%s.pkt", nd.Name, sanitizeAddrForFilename(destAddr), time.Now().Format("20060102150405.000000")))

		f, err := os.Create(pktName)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("create pkt %s: %v", pktName, err))
			continue
		}

		pmsgs := make([]*Message, len(entries))
		for i, be := range entries {
			pmsgs[i] = be.pmsg
		}

		password := nd.Password
		if err := WritePacket(f, orig, destAddr, password, pmsgs); err != nil {
			f.Close()
			result.Errors = append(result.Errors, fmt.Sprintf("write pkt %s: %v", pktName, err))
			continue
		}
		f.Close()
		result.PKTFiles++

		// Only now that the PKT is safely on disk do we mark these messages
		// exported — a write failure above leaves them unmarked so they are
		// retried on the next scan, instead of being silently lost.
		//
		// NOTE: a message fanned out to multiple destinations (uplink +
		// subscribed downlinks) is marked exported once its FIRST bucket
		// write succeeds, even if a later bucket (e.g. a downlink PKT)
		// fails — it will not be retried for that specific destination on
		// the next scan. This is an accepted simplification: exported
		// tracking is per-message, not per-(message, destination).
		for _, be := range entries {
			if err := store.MarkExported(be.id); err != nil {
				result.Errors = append(result.Errors,
					fmt.Sprintf("mark exported id=%d: %v", be.id, err))
			}
		}
	}

	return nil
}

// sanitizeAddrForFilename returns a filesystem-safe representation of addr
// for use in outbound PKT filenames (so each destination's packets are
// trivially distinguishable on disk).
func sanitizeAddrForFilename(addr Addr) string {
	return strings.NewReplacer(":", "z", "/", "n", ".", "p").Replace(addr.String())
}

// appendEchoMessage builds the packet-ready echo message for m, appends it
// to buckets[key] (the conference's uplink), and additionally fans it out
// to every downlink currently subscribed to areaTag via AreaFix.
func appendEchoMessage(buckets map[string][]bucketEntry, destAddrs map[string]Addr, key string, m *messages.Message,
	areaTag string, orig, dest Addr, bbsName string, taglines []string, nd *NetworkDef,
	areafixDB *AreaFixDB, result *ScanResult) {

	var inSeenBy, inPath []string
	if m.FidoSeenBy != "" {
		inSeenBy = strings.Fields(m.FidoSeenBy)
	}
	if m.FidoPath != "" {
		inPath = strings.Fields(m.FidoPath)
	}
	tagline := PickTagline(taglines)

	body := buildEchoBody(areaTag, orig, bbsName, m.Body, tagline, m.FidoMsgID, m.FidoReply, m.FidoKludges, inSeenBy, inPath)
	entry := bucketEntry{
		pmsg: &Message{
			OrigAddr: orig,
			DestAddr: dest,
			DateTime: m.DatePosted.Format("02 Jan 06  15:04:05"),
			ToName:   m.ToName,
			FromName: m.FromName,
			Subject:  m.Subject,
			Body:     body,
		},
		id: m.ID,
	}
	buckets[key] = append(buckets[key], entry)
	result.Scanned++

	if areafixDB == nil {
		return
	}
	downlinks, err := areafixDB.SubscribedDownlinks(nd.Name, areaTag)
	if err != nil || len(downlinks) == 0 {
		return
	}
	for _, addrStr := range downlinks {
		dlAddr, err := ParseAddr(addrStr)
		if err != nil || dlAddr == dest {
			continue // skip malformed entries and the case where a downlink IS the uplink
		}
		if nd.DownlinkByAddr(dlAddr) == nil {
			// Stale subscription left over after the downlink was removed
			// from config — skip it rather than mailing a no-longer-known
			// system (the sysop menu also deletes subscriptions on removal,
			// this is a defensive second check).
			continue
		}
		dlKey := dlAddr.String()
		destAddrs[dlKey] = dlAddr
		buckets[dlKey] = append(buckets[dlKey], bucketEntry{
			pmsg: &Message{
				OrigAddr: orig,
				DestAddr: dlAddr,
				DateTime: entry.pmsg.DateTime,
				ToName:   m.ToName,
				FromName: m.FromName,
				Subject:  m.Subject,
				Body:     body, // same bytes — this is the same echo message, just a second destination
			},
			id: m.ID,
		})
	}
}

// buildEchoBody constructs the full FTS-0004 echomail body:
//
//	AREA:<tag>
//	^AMSGID: <orig> <serial>
//	^AREPLY: <parent-msgid>  (when replying)
//	^ATZUTC: ±HHMM
//	<message text>
//	[blank line + tagline, if one is configured]
//	--- VirtBBS <version>
//	 * Origin: <bbsName> (<orig>)
//	SEEN-BY: <merged net/node list>
//	^APATH: <merged net/node list>
//
// msgID/reply are taken from the stored message when present so exported
// packets preserve threading. extraKludges holds other ^A lines (TZUTC, etc.).
func buildEchoBody(areaTag string, orig Addr, bbsName, body, tagline, msgID, reply, extraKludges string, inSeenBy, inPath []string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "AREA:%s\r", areaTag)
	if msgID == "" {
		msgID = FormatMSGID(orig, NewMSGIDSerial())
	}
	fmt.Fprintf(&sb, "\x01MSGID: %s\r", msgID)
	if reply != "" {
		fmt.Fprintf(&sb, "\x01REPLY: %s\r", reply)
	}
	if extraKludges != "" {
		for _, line := range strings.Split(extraKludges, "\r") {
			line = strings.TrimRight(line, "\n")
			if line != "" {
				sb.WriteString(line)
				sb.WriteString("\r")
			}
		}
	} else {
		fmt.Fprintf(&sb, "\x01TZUTC: %s\r", time.Now().Format("-0700"))
	}

	sb.WriteString(body)
	if !strings.HasSuffix(body, "\r") {
		sb.WriteString("\r")
	}

	if tagline != "" {
		fmt.Fprintf(&sb, "\r%s\r", tagline)
	}

	fmt.Fprintf(&sb, "--- VirtBBS %s\r", version.Version)
	fmt.Fprintf(&sb, " * Origin: %s (%s)\r", bbsName, orig.String())

	seenBy := MergeAddrTokens(inSeenBy, orig)
	fmt.Fprintf(&sb, "SEEN-BY: %s\r", strings.Join(seenBy, " "))

	path := MergeAddrTokens(inPath, orig)
	fmt.Fprintf(&sb, "\x01PATH: %s\r", strings.Join(path, " "))

	return sb.String()
}
