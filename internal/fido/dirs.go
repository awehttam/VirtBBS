package fido

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var networkDirSanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// SanitizeNetworkDirName turns a network display name into a safe path segment.
func SanitizeNetworkDirName(name string) string {
	s := strings.TrimSpace(name)
	if s == "" {
		return "network"
	}
	return networkDirSanitizer.ReplaceAllString(s, "_")
}

// DefaultDirsForNetwork returns default inbound/outbound/nodelist paths for a
// network. The primary network keeps the historical fido/inbound layout; others
// use fido/<Name>_inbound etc.
func DefaultDirsForNetwork(name string) (inbound, outbound, nodelist string) {
	if name == "" || strings.EqualFold(name, PrimaryNetworkName) {
		return "fido/inbound", "fido/outbound", "fido/nodelist"
	}
	safe := SanitizeNetworkDirName(name)
	return "fido/" + safe + "_inbound",
		"fido/" + safe + "_outbound",
		"fido/" + safe + "_nodelist"
}

// EffectiveHoldingDir returns where orphaned/skipped messages are held for
// review. Defaults to <inbound>/.holding unless HoldingDir is set.
func (n *NetworkDef) EffectiveHoldingDir() string {
	if n.HoldingDir != "" {
		return n.HoldingDir
	}
	if n.InboundDir != "" {
		return filepath.Join(n.InboundDir, ".holding")
	}
	return GlobalHoldingDir()
}

// GlobalHoldingDir is used when the source network cannot be determined.
const GlobalHoldingDirPath = "fido/holding"

func GlobalHoldingDir() string { return GlobalHoldingDirPath }

// EnsureNetworkDirs creates inbound, outbound, nodelist, .tossed, and
// .holding directories for one network definition.
func EnsureNetworkDirs(nd *NetworkDef) error {
	dirs := []string{nd.InboundDir, nd.OutboundDir, nd.NodelistDir}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	if nd.InboundDir != "" {
		for _, sub := range []string{".tossed", ".holding"} {
			if err := os.MkdirAll(filepath.Join(nd.InboundDir, sub), 0755); err != nil {
				return err
			}
		}
	}
	if nd.HoldingDir != "" {
		if err := os.MkdirAll(nd.HoldingDir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// EnsureAllNetworkDirs creates mail directories for every configured network.
func EnsureAllNetworkDirs(cfg *Config) error {
	if err := os.MkdirAll(GlobalHoldingDir(), 0755); err != nil {
		return err
	}
	for _, nd := range cfg.AllNetworks() {
		// Fill blank dirs on save so new networks get sensible defaults.
		in, out, nl := DefaultDirsForNetwork(nd.Name)
		if nd.InboundDir == "" {
			nd.InboundDir = in
		}
		if nd.OutboundDir == "" {
			nd.OutboundDir = out
		}
		if nd.NodelistDir == "" {
			nd.NodelistDir = nl
		}
		if err := EnsureNetworkDirs(&nd); err != nil {
			return err
		}
		// Write back filled dirs into cfg for primary vs secondary.
		cfg.applyNetworkDirs(&nd)
	}
	return nil
}

func (c *Config) applyNetworkDirs(nd *NetworkDef) {
	if nd.Name == "" || strEqFold(nd.Name, c.EffectivePrimaryName()) {
		if nd.InboundDir != "" {
			c.InboundDir = nd.InboundDir
		}
		if nd.OutboundDir != "" {
			c.OutboundDir = nd.OutboundDir
		}
		if nd.NodelistDir != "" {
			c.NodelistDir = nd.NodelistDir
		}
		if nd.HoldingDir != "" {
			c.HoldingDir = nd.HoldingDir
		}
		return
	}
	for i := range c.Networks {
		if strEqFold(c.Networks[i].Name, nd.Name) {
			c.Networks[i].InboundDir = nd.InboundDir
			c.Networks[i].OutboundDir = nd.OutboundDir
			c.Networks[i].NodelistDir = nd.NodelistDir
			c.Networks[i].HoldingDir = nd.HoldingDir
			return
		}
	}
}
