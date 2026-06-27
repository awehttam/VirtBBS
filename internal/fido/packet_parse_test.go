package fido

import "testing"

func TestParse_MSGID_REPLY_Kludges(t *testing.T) {
	body := "AREA:GENERAL\r" +
		"\x01MSGID: 1:234/1 ABCD1234\r" +
		"\x01REPLY: 1:234/2 DEADBEEF\r" +
		"\x01TZUTC: -0500\r" +
		"Hello world\r" +
		"--- tear\r" +
		" * Origin: Test (1:234/1)\r" +
		"SEEN-BY: 234/1\r" +
		"\x01PATH: 234/1\r"

	pb := (&Message{Body: body}).Parse()

	if pb.AreaTag != "GENERAL" {
		t.Errorf("AreaTag = %q, want GENERAL", pb.AreaTag)
	}
	if pb.MSGID != "1:234/1 ABCD1234" {
		t.Errorf("MSGID = %q", pb.MSGID)
	}
	if pb.REPLY != "1:234/2 DEADBEEF" {
		t.Errorf("REPLY = %q", pb.REPLY)
	}
	if pb.Kludges != "\x01TZUTC: -0500" {
		t.Errorf("Kludges = %q, want TZUTC line", pb.Kludges)
	}
	if len(pb.SeenBy) != 1 || pb.SeenBy[0] != "234/1" {
		t.Errorf("SeenBy = %v", pb.SeenBy)
	}
	if len(pb.Path) != 1 || pb.Path[0] != "234/1" {
		t.Errorf("Path = %v", pb.Path)
	}
	if pb.Text == "" || pb.Text == body {
		t.Errorf("Text should be cleaned body, got %q", pb.Text)
	}
}

func TestBuildEchoBody_preservesThreadKludges(t *testing.T) {
	orig := Addr{Zone: 1, Net: 234, Node: 1}
	body := buildEchoBody("GENERAL", orig, "Test BBS", "Hi\r", "",
		"1:234/1 00000001", "1:234/1 2 ABCD1234", "\x01TZUTC: -0500", nil, nil)

	if !containsSubstr(body, "\x01MSGID: 1:234/1 00000001") {
		t.Errorf("missing stored MSGID in %q", body)
	}
	if !containsSubstr(body, "\x01REPLY: 1:234/1 2 ABCD1234") {
		t.Errorf("missing REPLY in %q", body)
	}
	if !containsSubstr(body, "\x01TZUTC: -0500") {
		t.Errorf("missing stored TZUTC in %q", body)
	}
}

func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
