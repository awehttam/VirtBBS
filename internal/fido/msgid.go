package fido

import (
	"fmt"
	"time"
)

// FormatMSGID builds an FTS-0009 MSGID value: "<addr> <8-hex-serial>".
func FormatMSGID(orig Addr, serial uint32) string {
	return fmt.Sprintf("%s %08X", orig.String(), serial)
}

// NewMSGIDSerial returns a pseudo-unique 32-bit serial for local MSGIDs.
func NewMSGIDSerial() uint32 {
	return uint32(time.Now().UnixNano() & 0xFFFFFFFF)
}
