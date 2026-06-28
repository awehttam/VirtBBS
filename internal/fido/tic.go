package fido

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
)

// TICTicket is a parsed FTS-5006-style TIC control file.
type TICTicket struct {
	Area     string
	Origin   string
	From     string
	File     string
	Desc     string
	Size     int64
	CRC      string
	Path     string
	SeenBy   string
	Password string
}

// ParseTIC parses TIC file body (CR/LF lines, "Keyword value").
func ParseTIC(data []byte) (*TICTicket, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	t := &TICTicket{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])
		switch key {
		case "area":
			t.Area = strings.ToUpper(val)
		case "origin":
			t.Origin = val
		case "from":
			t.From = val
		case "file":
			t.File = val
		case "desc":
			t.Desc = val
		case "size":
			t.Size, _ = strconv.ParseInt(val, 10, 64)
		case "crc":
			t.CRC = strings.ToUpper(val)
		case "path":
			t.Path = val
		case "seenby":
			t.SeenBy = val
		case "pw":
			t.Password = val
		}
	}
	if t.Area == "" || t.File == "" {
		return nil, fmt.Errorf("tic: missing Area or File")
	}
	if t.From == "" {
		t.From = t.Origin
	}
	return t, nil
}

// FormatTIC serialises a ticket using CRLF line endings.
func FormatTIC(t *TICTicket) []byte {
	var b strings.Builder
	writeTICLine(&b, "Area", t.Area)
	if t.Origin != "" {
		writeTICLine(&b, "Origin", t.Origin)
	}
	if t.From != "" {
		writeTICLine(&b, "From", t.From)
	}
	writeTICLine(&b, "File", t.File)
	if t.Desc != "" {
		writeTICLine(&b, "Desc", t.Desc)
	}
	if t.Size > 0 {
		writeTICLine(&b, "Size", strconv.FormatInt(t.Size, 10))
	}
	if t.CRC != "" {
		writeTICLine(&b, "CRC", t.CRC)
	}
	if t.Path != "" {
		writeTICLine(&b, "Path", t.Path)
	}
	if t.SeenBy != "" {
		writeTICLine(&b, "SeenBy", t.SeenBy)
	}
	if t.Password != "" {
		writeTICLine(&b, "Pw", t.Password)
	}
	b.WriteString("\r\n")
	return []byte(b.String())
}

func writeTICLine(b *strings.Builder, key, val string) {
	fmt.Fprintf(b, "%s %s\r\n", key, val)
}

// TICFileCRC returns the uppercase 8-digit CRC-32 (IEEE) used by FidoNet TIC.
func TICFileCRC(data []byte) string {
	return fmt.Sprintf("%08X", crc32.ChecksumIEEE(data))
}
