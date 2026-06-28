package fido

import (
	"bytes"
	"testing"
)

func TestParseFormatTIC(t *testing.T) {
	orig := &TICTicket{
		Area: "GAMES", Origin: "1:153/150", From: "1:153/150",
		File: "DEMO.ZIP", Desc: "Demo", Size: 1024, CRC: "DEADBEEF",
		Path: "153/150", SeenBy: "153/150", Password: "secret",
	}
	body := FormatTIC(orig)
	got, err := ParseTIC(body)
	if err != nil {
		t.Fatal(err)
	}
	if got.Area != orig.Area || got.File != orig.File || got.CRC != orig.CRC {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestTICFileCRC(t *testing.T) {
	data := []byte("hello")
	if TICFileCRC(data) != "3610A686" {
		t.Fatalf("unexpected CRC %s", TICFileCRC(data))
	}
}

func TestParseTICRequiresAreaFile(t *testing.T) {
	if _, err := ParseTIC([]byte("Desc test\r\n")); err == nil {
		t.Fatal("expected error for incomplete TIC")
	}
}

func TestFormatTICEndsWithCRLF(t *testing.T) {
	body := FormatTIC(&TICTicket{Area: "X", File: "Y.ZIP", CRC: "00000000"})
	if !bytes.HasSuffix(body, []byte("\r\n")) {
		t.Fatal("expected CRLF ending")
	}
}
