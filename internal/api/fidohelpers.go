package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

func randomMemberPassword() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "changeme"
	}
	return hex.EncodeToString(buf)
}

func saveNetworkDownlink(networkName string, dl fido.Downlink) error {
	cfg := config.Get()
	merged := *cfg
	if strings.EqualFold(networkName, cfg.Fido.EffectivePrimaryName()) {
		merged.Fido.Downlinks = append(append([]fido.Downlink{}, cfg.Fido.Downlinks...), dl)
		return config.Save(&merged)
	}
	merged.Fido.Networks = append([]fido.NetworkDef{}, cfg.Fido.Networks...)
	for i := range merged.Fido.Networks {
		if strings.EqualFold(merged.Fido.Networks[i].Name, networkName) {
			merged.Fido.Networks[i].Downlinks = append(
				append([]fido.Downlink{}, merged.Fido.Networks[i].Downlinks...), dl)
			return config.Save(&merged)
		}
	}
	return fmt.Errorf("network %q not found", networkName)
}

func networkDef(name string) (*fido.NetworkDef, error) {
	cfg := config.Get()
	nd := cfg.Fido.NetworkByName(name)
	if nd == nil {
		return nil, fmt.Errorf("network %q not found", name)
	}
	return nd, nil
}
