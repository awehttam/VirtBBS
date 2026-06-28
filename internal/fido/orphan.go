package fido

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/virtbbs/virtbbs/internal/messages"
)

// OrphanNote describes one message held for sysop review.
type OrphanNote struct {
	Reason  string `json:"reason"`
	Area    string `json:"area"`
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	File    string `json:"file"`
}

// MatchesOurAddr reports whether addr is one of this node's addresses
// (ignoring point, per standard netmail delivery).
func (n *NetworkDef) MatchesOurAddr(addr Addr) bool {
	for _, known := range n.AllAddrs() {
		if known.Zone == addr.Zone && known.Net == addr.Net && known.Node == addr.Node {
			return true
		}
	}
	return false
}

// HoldOrphanMessage writes a single message to the network holding directory
// as a one-message .PKT and appends a line to ORPHANS.log.
func HoldOrphanMessage(nd *NetworkDef, pm *Message, reason string) (OrphanNote, error) {
	dir := nd.EffectiveHoldingDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return OrphanNote{}, err
	}

	area := pm.Parse().AreaTag
	base := fmt.Sprintf("orphan_%s_%d.pkt", time.Now().Format("20060102_150405"), time.Now().UnixNano()%100000)
	path := filepath.Join(dir, base)

	f, err := os.Create(path)
	if err != nil {
		return OrphanNote{}, err
	}
	defer f.Close()

	our := nd.NodeAddr()
	if our == (Addr{}) {
		our = pm.DestAddr
	}
	if err := WritePacket(f, pm.OrigAddr, our, nd.Password, []*Message{pm}); err != nil {
		_ = os.Remove(path)
		return OrphanNote{}, err
	}

	note := OrphanNote{
		Reason:  reason,
		Area:    area,
		From:    pm.FromName,
		To:      pm.ToName,
		Subject: pm.Subject,
		File:    path,
	}
	appendOrphanLog(dir, note)
	return note, nil
}

func appendOrphanLog(dir string, note OrphanNote) {
	logPath := filepath.Join(dir, "ORPHANS.log")
	line := fmt.Sprintf("%s\t%s\tarea=%q\tfrom=%q\tto=%q\tsubject=%q\tfile=%s\r\n",
		time.Now().Format(time.RFC3339), note.Reason, note.Area, note.From, note.To, note.Subject, note.File)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// NotifySysopOrphans posts a summary netmail to the sysop in conference 0.
func NotifySysopOrphans(store *messages.Store, sysopName, network string, notes []OrphanNote) error {
	if len(notes) == 0 || sysopName == "" {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Orphaned mail held for review on network %s.\r\n\r\n", network)
	for _, n := range notes {
		fmt.Fprintf(&b, "  [%s] ", n.Reason)
		if n.Area != "" {
			fmt.Fprintf(&b, "area %s ", n.Area)
		}
		fmt.Fprintf(&b, "from %s to %s: %s\r\n    → %s\r\n", n.From, n.To, n.Subject, n.File)
	}
	fmt.Fprintf(&b, "\r\nReview files under the network inbound .holding directory.\r\n")

	m := &messages.Message{
		ConferenceID: 0,
		FromName:     "Mail Processor",
		ToName:       sysopName,
		Subject:      fmt.Sprintf("Orphaned mail held [%s]", network),
		DatePosted:   time.Now(),
		Status:       "A",
		Body:         b.String(),
	}
	return store.Post(m)
}
