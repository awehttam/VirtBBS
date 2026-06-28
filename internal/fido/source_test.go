package fido

import (
	"strings"
	"testing"
)

func TestReconstructSource_echoRoundTrip(t *testing.T) {
	original := "AREA:GENERAL\r" +
		"\x01MSGID: 1:234/1 ABCD1234\r" +
		"\x01REPLY: 1:234/2 DEADBEEF\r" +
		"\x01TZUTC: -0500\r" +
		"\x01LANG: EN\r" +
		"Hello world\r" +
		"--- tear\r" +
		" * Origin: Test (1:234/1)\r" +
		"SEEN-BY: 234/1 234/2\r" +
		"\x01PATH: 234/1 234/2\r"

	pb := (&Message{Body: original}).Parse()
	got := ReconstructSource(SourceOpts{
		Body:        pb.Text,
		FidoMsgID:   pb.MSGID,
		FidoReply:   pb.REPLY,
		FidoKludges: pb.Kludges,
		FidoSeenBy:  strings.Join(pb.SeenBy, " "),
		FidoPath:    strings.Join(pb.Path, " "),
		AreaTag:     pb.AreaTag,
	})

	for _, want := range []string{
		"AREA:GENERAL\r",
		"\x01MSGID: 1:234/1 ABCD1234\r",
		"\x01REPLY: 1:234/2 DEADBEEF\r",
		"\x01TZUTC: -0500\r",
		"\x01LANG: EN\r",
		"Hello world\r",
		"--- tear\r",
		" * Origin: Test (1:234/1)\r",
		"SEEN-BY: 234/1 234/2\r",
		"\x01PATH: 234/1 234/2\r",
	} {
		if !containsSubstr(got, want) {
			t.Errorf("ReconstructSource missing %q in:\n%s", want, got)
		}
	}
}

func TestReconstructSource_netmail(t *testing.T) {
	original := "\x01MSGID: 1:234/1 NETMAIL01\r" +
		"\x01TZUTC: +0200\r" +
		"Private note\r" +
		"SEEN-BY: 234/1\r" +
		"\x01PATH: 234/1\r"

	pb := (&Message{Body: original}).Parse()
	got := ReconstructSource(SourceOpts{
		Body:        pb.Text,
		FidoMsgID:   pb.MSGID,
		FidoKludges: pb.Kludges,
		FidoSeenBy:  strings.Join(pb.SeenBy, " "),
		FidoPath:    strings.Join(pb.Path, " "),
	})

	if containsSubstr(got, "AREA:") {
		t.Errorf("netmail source should not include AREA:, got %q", got)
	}
	for _, want := range []string{
		"\x01MSGID: 1:234/1 NETMAIL01\r",
		"\x01TZUTC: +0200\r",
		"Private note\r",
		"SEEN-BY: 234/1\r",
		"\x01PATH: 234/1\r",
	} {
		if !containsSubstr(got, want) {
			t.Errorf("ReconstructSource missing %q in:\n%s", want, got)
		}
	}
}

func TestReconstructSource_minimalBodyOnly(t *testing.T) {
	got := ReconstructSource(SourceOpts{Body: "Just text"})
	if got != "Just text\r" {
		t.Errorf("got %q, want %q", got, "Just text\r")
	}
}
