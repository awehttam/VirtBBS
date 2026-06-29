package web

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
)

func fidoNetworkNamesList() []string {
	cfg := config.Get()
	primary := cfg.Fido.EffectivePrimaryName()
	names := []string{primary}
	for _, nd := range cfg.Fido.Networks {
		if nd.Name != "" && !strings.EqualFold(nd.Name, primary) {
			names = append(names, nd.Name)
		}
	}
	return names
}

func networkDefByName(name string) (*fido.NetworkDef, error) {
	cfg := config.Get()
	nd := cfg.Fido.NetworkByName(name)
	if nd == nil {
		return nil, fmt.Errorf("network %q not found", name)
	}
	return nd, nil
}

func importNodelistRestoreLocal(db *sql.DB, path, network string) (*fido.ImportResult, error) {
	result, err := fido.ImportFile(db, path, network)
	if err != nil {
		return result, err
	}
	restoreLocalNodelistEntries(db, network)
	return result, nil
}

func restoreLocalNodelistEntries(db *sql.DB, network string) {
	cfg := config.Get()
	nd := cfg.Fido.NetworkByName(network)
	if nd == nil {
		return
	}
	_ = fido.RestoreLocalNodeEntries(db, nd, cfg.BBS.Name, cfg.Sysop.Name, "Internet", cfg.Network.TelnetPort)
}

func linkNodelistAKAs(nodes []*fido.NodeEntry, network string) {
	if len(nodes) == 0 {
		return
	}
	fido.LinkHostAKAsPtrs(nodes)
	if nd, err := networkDefByName(network); err == nil {
		fido.LinkConfiguredAKAs(nodes, nd)
	}
}

func saveFidoConfig(db *sql.DB, fidoCfg fido.Config) error {
	current := config.Get()
	merged := *current
	merged.Fido = fidoCfg
	if err := fido.EnsureAllNetworkDirs(&merged.Fido); err != nil {
		return err
	}
	if err := config.Save(&merged); err != nil {
		return err
	}
	cfg := config.Get()
	fido.EnsureAllNetworkOwnNodes(db, cfg.Fido.AllNetworks(),
		cfg.BBS.Name, cfg.Sysop.Name, cfg.Network.TelnetPort)
	return nil
}

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

func updateNetworkDownlinks(networkName string, mutate func([]fido.Downlink) []fido.Downlink) error {
	cfg := config.Get()
	merged := *cfg
	if strings.EqualFold(networkName, cfg.Fido.EffectivePrimaryName()) {
		merged.Fido.Downlinks = mutate(append([]fido.Downlink{}, cfg.Fido.Downlinks...))
		return config.Save(&merged)
	}
	merged.Fido.Networks = append([]fido.NetworkDef{}, cfg.Fido.Networks...)
	for i := range merged.Fido.Networks {
		if strings.EqualFold(merged.Fido.Networks[i].Name, networkName) {
			merged.Fido.Networks[i].Downlinks = mutate(
				append([]fido.Downlink{}, merged.Fido.Networks[i].Downlinks...))
			return config.Save(&merged)
		}
	}
	return fmt.Errorf("network %q not found", networkName)
}

func removeNetworkDownlink(db *sql.DB, networkName, addr string) (bool, error) {
	removed := false
	err := updateNetworkDownlinks(networkName, func(cur []fido.Downlink) []fido.Downlink {
		var kept []fido.Downlink
		for _, dl := range cur {
			if strings.EqualFold(dl.Address, addr) {
				removed = true
				continue
			}
			kept = append(kept, dl)
		}
		return kept
	})
	if err != nil || !removed {
		return removed, err
	}
	areafixDB := fido.OpenAreaFixDB(db)
	tags, _ := areafixDB.SubscriptionsFor(networkName, addr)
	for _, tag := range tags {
		_ = areafixDB.Unsubscribe(networkName, addr, tag)
	}
	filefixDB := fido.OpenFileFixDB(db)
	ftags, _ := filefixDB.SubscriptionsFor(networkName, addr)
	for _, tag := range ftags {
		_ = filefixDB.Unsubscribe(networkName, addr, tag)
	}
	return true, nil
}

func saveNetworkNodeFlags(networkName string, flags []string, binkpHost string) error {
	cfg := config.Get()
	merged := *cfg
	if strings.EqualFold(networkName, cfg.Fido.EffectivePrimaryName()) {
		merged.Fido.NodeFlags = flags
		merged.Fido.BinkpHost = binkpHost
		return config.Save(&merged)
	}
	merged.Fido.Networks = append([]fido.NetworkDef{}, cfg.Fido.Networks...)
	for i := range merged.Fido.Networks {
		if strings.EqualFold(merged.Fido.Networks[i].Name, networkName) {
			merged.Fido.Networks[i].NodeFlags = flags
			merged.Fido.Networks[i].BinkpHost = binkpHost
			return config.Save(&merged)
		}
	}
	return fmt.Errorf("network %q not found", networkName)
}

func parseAreaMapLines(text string) map[string]int {
	out := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		tag := strings.TrimSpace(parts[0])
		id, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		if tag != "" && id > 0 {
			out[tag] = id
		}
	}
	return out
}

func formatAreaMap(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	var lines []string
	for tag, id := range m {
		lines = append(lines, fmt.Sprintf("%s=%d", tag, id))
	}
	return strings.Join(lines, "\n")
}

func parseDownlinkLines(text string) []fido.Downlink {
	var out []fido.Downlink
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}
		dl := fido.Downlink{Name: strings.TrimSpace(parts[0]), Address: strings.TrimSpace(parts[1])}
		if len(parts) >= 3 {
			dl.Password = strings.TrimSpace(parts[2])
		}
		out = append(out, dl)
	}
	return out
}

func formatDownlinks(dls []fido.Downlink) string {
	if len(dls) == 0 {
		return ""
	}
	var lines []string
	for _, dl := range dls {
		lines = append(lines, fmt.Sprintf("%s|%s|%s", dl.Name, dl.Address, dl.Password))
	}
	return strings.Join(lines, "\n")
}

func formInt(r *http.Request, key string, def int) int {
	v, err := strconv.Atoi(strings.TrimSpace(r.FormValue(key)))
	if err != nil {
		return def
	}
	return v
}

func formBool(r *http.Request, key string) bool {
	v := strings.TrimSpace(r.FormValue(key))
	return v == "1" || strings.EqualFold(v, "on") || strings.EqualFold(v, "true")
}

func selectedNetwork(r *http.Request) string {
	n := strings.TrimSpace(r.FormValue("network"))
	if n == "" {
		n = strings.TrimSpace(r.URL.Query().Get("network"))
	}
	if n == "" {
		names := fidoNetworkNamesList()
		if len(names) > 0 {
			n = names[0]
		}
	}
	return n
}
