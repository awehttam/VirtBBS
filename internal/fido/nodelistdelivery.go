package fido

import (
	"database/sql"
	"strings"
)

// IsHoldOrDownType reports whether a nodelist entry type means the node should
// not be dialed — mail is held until the node polls the host.
func IsHoldOrDownType(nodeType string) bool {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "hold", "down":
		return true
	default:
		return false
	}
}

// NodelistShouldDeferDelivery reports whether outbound routing to addr should
// not target that system directly (crash or indirect .OUT delivery). Mail is
// queued for the uplink instead and delivered when the destination polls in.
//
// Configured downlinks are exempt: they always pick up tagged mail when they
// poll this BBS.
func NodelistShouldDeferDelivery(db *sql.DB, nd *NetworkDef, addr Addr) bool {
	if nd != nil && nd.DownlinkByAddr(addr) != nil {
		return false
	}
	if db == nil || nd == nil {
		return false
	}
	ndb := OpenNodelistDB(db)
	entry, err := ndb.LookupAddr(nd.Name, addr)
	if err != nil || entry == nil {
		return false
	}
	return IsHoldOrDownType(entry.Type)
}
