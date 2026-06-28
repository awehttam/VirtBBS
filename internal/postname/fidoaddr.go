package postname

import (
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

// EchoOrigAddr returns the FidoNet node address used as MSGID origin for posts
// in the given conference, or Addr{} when FidoNet is disabled.
func EchoOrigAddr(c *conferences.Conference) fido.Addr {
	cfg := config.Get()
	if !cfg.Fido.Enabled {
		return fido.Addr{}
	}
	orig := cfg.Fido.NodeAddr()
	if c != nil && c.Network != "" {
		if nd := cfg.Fido.NetworkByName(c.Network); nd != nil {
			if a := nd.NodeAddr(); a != (fido.Addr{}) {
				orig = a
			}
		}
	}
	return orig
}
