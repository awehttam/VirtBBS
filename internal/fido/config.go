// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//
// Change History:
//   v0.0.3  2026-06-24  Phase 9: FidoNet area configuration
//   v0.0.5  2026-06-24  Add json tags so API returns clean lowercase keys
//   v0.0.6  2026-06-24  Add NodelistPath, BinkpPort, Networks for multi-network support
//   v0.1.0  2026-06-25  Add TaglinesFile for the configurable echomail tagline feature
//   v0.2.0  2026-06-25  Add Downlinks + AreaFixPassword for the AreaFix responder/requester
//   v0.3.0  2026-06-25  Add PollIntervalMins for the automatic poll/toss scheduler
//   v0.5.0  2026-06-25  Add NodelistURL/NodelistUpdateIntervalHours for automatic
//                        nodelist fetching (see internal/fido/nodelistfetch.go)
// ============================================================================

package fido

import "time"

// Config holds all FidoNet settings for VirtBBS.
// The top-level fields describe the primary (first) network.
// Additional networks are listed in Networks[].
type Config struct {
	Enabled     bool           `toml:"enabled"      json:"enabled"`
	Address     string         `toml:"address"      json:"address"`
	Uplink      string         `toml:"uplink"       json:"uplink"`
	Password    string         `toml:"password"     json:"password"`
	InboundDir  string         `toml:"inbound_dir"  json:"inbound_dir"`
	OutboundDir string         `toml:"outbound_dir" json:"outbound_dir"`
	NodelistDir string         `toml:"nodelist_dir" json:"nodelist_dir"` // dir holding NODELIST.xxx
	BinkpPort   int            `toml:"binkp_port"   json:"binkp_port"`   // default 24554
	Areas       map[string]int `toml:"areas"        json:"areas"`

	// TaglinesFile is an optional path to a text file with one tagline per
	// line. A line is chosen at random and inserted above the tear line on
	// each outgoing echomail message. Empty/unset means no taglines.
	TaglinesFile string `toml:"taglines_file" json:"taglines_file"`

	// Downlinks are systems that request echomail areas from this BBS via
	// AreaFix. Each entry's Password is what that downlink must supply (as
	// the first non-blank line of its netmail body) to manage its own
	// subscriptions — see internal/fido/areafix.go.
	Downlinks []Downlink `toml:"downlinks" json:"downlinks"`

	// AreaFixPassword is the password THIS BBS sends when requesting areas
	// from its own uplink's AreaFix (i.e. when VirtBBS acts as a downlink).
	AreaFixPassword string `toml:"areafix_password" json:"areafix_password"`

	// PollIntervalMins overrides how often the scheduler automatically polls
	// this network's uplink, in minutes. 0/unset means the scheduler default
	// of 6 hours. Any positive value is clamped to a 5-minute minimum.
	PollIntervalMins int `toml:"poll_interval_mins" json:"poll_interval_mins"`

	// FileAreas maps FileFix file-area tags to local file directory IDs
	// (internal/files.Dir.ID) — the FileFix equivalent of Areas. See
	// internal/fido/filefix.go.
	FileAreas map[string]int `toml:"file_areas" json:"file_areas"`

	// FileFixPassword is the password THIS BBS sends when requesting file
	// areas from its own uplink's FileFix — the FileFix equivalent of
	// AreaFixPassword.
	FileFixPassword string `toml:"filefix_password" json:"filefix_password"`

	// NodelistURL is where the automatic nodelist fetcher downloads from.
	// May be a direct file URL, or (the default if left blank) a discovery
	// page that gets scanned for today's "Fidonet Daily Nodelist (Z1/ZIP)
	// day NNN" link — see internal/fido/nodelistfetch.go and
	// DefaultNodelistDiscoveryURL.
	NodelistURL string `toml:"nodelist_url" json:"nodelist_url"`

	// NodelistUpdateIntervalHours overrides how often the scheduler
	// automatically fetches a fresh nodelist, in hours. 0/unset means the
	// scheduler default of 24 hours. Any positive value is clamped to a
	// 1-hour minimum.
	NodelistUpdateIntervalHours int `toml:"nodelist_update_interval_hours" json:"nodelist_update_interval_hours"`

	// Networks lists additional FidoNet-compatible networks (LovlyNet, etc.).
	// Each entry is a fully independent network with its own address space.
	Networks []NetworkDef `toml:"networks" json:"networks"`
}

// DefaultNodelistDiscoveryURL is used when NodelistURL is left blank: a
// page listing daily FidoNet Zone 1 nodelist downloads, scanned for the
// "Fidonet Daily Nodelist (Z1/ZIP) day NNN" link (the actual file href
// changes daily and isn't derivable from a fixed pattern, so the page is
// scanned fresh on every fetch — see internal/fido/nodelistfetch.go).
const DefaultNodelistDiscoveryURL = "https://www.darkrealms.ca/"

// Downlink describes one system that subscribes to echomail areas from this
// BBS via AreaFix (case-insensitive netmail to "AreaFix").
type Downlink struct {
	Name     string `toml:"name"     json:"name"`     // sysop/system name, for display only
	Address  string `toml:"address"  json:"address"`  // zone:net/node of the downlink
	Password string `toml:"password" json:"password"` // password the downlink must supply
}

// NetworkDef describes one additional FidoNet-compatible network.
type NetworkDef struct {
	Name        string         `toml:"name"         json:"name"`
	Enabled     bool           `toml:"enabled"      json:"enabled"`
	Address     string         `toml:"address"      json:"address"`
	Uplink      string         `toml:"uplink"       json:"uplink"`
	Password    string         `toml:"password"     json:"password"`
	InboundDir  string         `toml:"inbound_dir"  json:"inbound_dir"`
	OutboundDir string         `toml:"outbound_dir" json:"outbound_dir"`
	NodelistDir string         `toml:"nodelist_dir" json:"nodelist_dir"`
	BinkpPort   int            `toml:"binkp_port"   json:"binkp_port"`
	Areas       map[string]int `toml:"areas"        json:"areas"`

	// TaglinesFile — see Config.TaglinesFile. Falls back to the primary
	// network's TaglinesFile in AllNetworks() if left blank.
	TaglinesFile string `toml:"taglines_file" json:"taglines_file"`

	// Downlinks/AreaFixPassword/PollIntervalMins/FileAreas/FileFixPassword/
	// NodelistURL/NodelistUpdateIntervalHours — see the matching Config fields.
	Downlinks                   []Downlink     `toml:"downlinks" json:"downlinks"`
	AreaFixPassword             string         `toml:"areafix_password" json:"areafix_password"`
	PollIntervalMins            int            `toml:"poll_interval_mins" json:"poll_interval_mins"`
	FileAreas                   map[string]int `toml:"file_areas" json:"file_areas"`
	FileFixPassword             string         `toml:"filefix_password" json:"filefix_password"`
	NodelistURL                 string         `toml:"nodelist_url" json:"nodelist_url"`
	NodelistUpdateIntervalHours int            `toml:"nodelist_update_interval_hours" json:"nodelist_update_interval_hours"`

	// NodelistEchoTag is the echo area tag this network's own generated
	// nodelist (when this BBS is the network's hub, Uplink=="") is
	// distributed under — see internal/fido/nodelistecho.go. Defaults to
	// DefaultNodelistEchoTag if unset.
	NodelistEchoTag string `toml:"nodelist_echo_tag" json:"nodelist_echo_tag"`
}

// DefaultConfig returns a Config with sensible disabled defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Address:     "1:1/1",
		InboundDir:  "fido/inbound",
		OutboundDir: "fido/outbound",
		NodelistDir: "fido/nodelist",
		BinkpPort:   24554,
		Areas:       map[string]int{},
	}
}

// NodeAddr parses this node's configured address.
func (c *Config) NodeAddr() Addr {
	if !c.Enabled || c.Address == "" {
		return Addr{}
	}
	a, _ := ParseAddr(c.Address)
	return a
}

// UplinkAddr parses the uplink address.
func (c *Config) UplinkAddr() Addr {
	if c.Uplink == "" {
		return Addr{}
	}
	a, _ := ParseAddr(c.Uplink)
	return a
}

// DownlinkByAddr finds a configured Downlink by address (ignoring point),
// or nil if addr isn't a known downlink.
func (c *Config) DownlinkByAddr(addr Addr) *Downlink {
	for i := range c.Downlinks {
		a, err := ParseAddr(c.Downlinks[i].Address)
		if err != nil {
			continue
		}
		if a.Zone == addr.Zone && a.Net == addr.Net && a.Node == addr.Node {
			return &c.Downlinks[i]
		}
	}
	return nil
}

// ConferenceForArea returns the conference ID mapped to an area tag, -1 if not found.
func (c *Config) ConferenceForArea(tag string) int {
	id, ok := c.Areas[tag]
	if !ok {
		return -1
	}
	return id
}

// FileDirForTag returns the file directory ID mapped to a FileFix tag, -1 if not found.
func (c *Config) FileDirForTag(tag string) int {
	id, ok := c.FileAreas[tag]
	if !ok {
		return -1
	}
	return id
}

// AllNetworks returns the primary network plus all additional networks as
// a flat slice of NetworkDef. Used when iterating all configured networks.
func (c *Config) AllNetworks() []NetworkDef {
	primary := NetworkDef{
		Name:            "FidoNet",
		Enabled:         c.Enabled,
		Address:         c.Address,
		Uplink:          c.Uplink,
		Password:        c.Password,
		InboundDir:      c.InboundDir,
		OutboundDir:     c.OutboundDir,
		NodelistDir:     c.NodelistDir,
		BinkpPort:        c.BinkpPort,
		Areas:            c.Areas,
		TaglinesFile:     c.TaglinesFile,
		Downlinks:        c.Downlinks,
		AreaFixPassword:  c.AreaFixPassword,
		PollIntervalMins:            c.PollIntervalMins,
		FileAreas:                   c.FileAreas,
		FileFixPassword:             c.FileFixPassword,
		NodelistURL:                 c.NodelistURL,
		NodelistUpdateIntervalHours: c.NodelistUpdateIntervalHours,
	}
	result := []NetworkDef{primary}
	result = append(result, c.Networks...)
	return result
}

// NetworkByName finds a NetworkDef by name (case-insensitive).
// Returns the primary network for an empty name.
func (c *Config) NetworkByName(name string) *NetworkDef {
	all := c.AllNetworks()
	for i := range all {
		if name == "" || strEqFold(all[i].Name, name) {
			return &all[i]
		}
	}
	return nil
}

func strEqFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// NetworkDef helpers

func (n *NetworkDef) NodeAddr() Addr {
	if n.Address == "" {
		return Addr{}
	}
	a, _ := ParseAddr(n.Address)
	return a
}

func (n *NetworkDef) UplinkAddr() Addr {
	if n.Uplink == "" {
		return Addr{}
	}
	a, _ := ParseAddr(n.Uplink)
	return a
}

func (n *NetworkDef) ConferenceForArea(tag string) int {
	id, ok := n.Areas[tag]
	if !ok {
		return -1
	}
	return id
}

// FileDirForTag returns the file directory ID mapped to a FileFix tag, -1 if not found.
func (n *NetworkDef) FileDirForTag(tag string) int {
	id, ok := n.FileAreas[tag]
	if !ok {
		return -1
	}
	return id
}

func (n *NetworkDef) Port() int {
	if n.BinkpPort <= 0 {
		return 24554
	}
	return n.BinkpPort
}

// DefaultPollInterval is how often the scheduler polls a network's uplink
// when PollIntervalMins is unset (0).
const DefaultPollInterval = 6 * time.Hour

// MinPollInterval is the smallest poll interval the scheduler will honour,
// regardless of what a sysop configures via PollIntervalMins.
const MinPollInterval = 5 * time.Minute

// EffectivePollInterval returns how often the scheduler should poll this
// network's uplink: PollIntervalMins if set, clamped to a 5-minute minimum,
// otherwise DefaultPollInterval (6 hours).
func (n *NetworkDef) EffectivePollInterval() time.Duration {
	if n.PollIntervalMins <= 0 {
		return DefaultPollInterval
	}
	d := time.Duration(n.PollIntervalMins) * time.Minute
	if d < MinPollInterval {
		return MinPollInterval
	}
	return d
}

// DefaultNodelistInterval is how often the scheduler fetches a fresh
// nodelist when NodelistUpdateIntervalHours is unset (0).
const DefaultNodelistInterval = 24 * time.Hour

// MinNodelistInterval is the smallest nodelist fetch interval the
// scheduler will honour, regardless of sysop configuration.
const MinNodelistInterval = 1 * time.Hour

// EffectiveNodelistInterval returns how often the scheduler should fetch a
// fresh nodelist for this network: NodelistUpdateIntervalHours if set,
// clamped to a 1-hour minimum, otherwise DefaultNodelistInterval (24h).
func (n *NetworkDef) EffectiveNodelistInterval() time.Duration {
	if n.NodelistUpdateIntervalHours <= 0 {
		return DefaultNodelistInterval
	}
	d := time.Duration(n.NodelistUpdateIntervalHours) * time.Hour
	if d < MinNodelistInterval {
		return MinNodelistInterval
	}
	return d
}

// EffectiveNodelistURL returns NodelistURL if configured, otherwise
// DefaultNodelistDiscoveryURL.
func (n *NetworkDef) EffectiveNodelistURL() string {
	if n.NodelistURL != "" {
		return n.NodelistURL
	}
	return DefaultNodelistDiscoveryURL
}

// DefaultNodelistEchoTag is used when NodelistEchoTag is left blank.
const DefaultNodelistEchoTag = "VNET.NODELIST"

// EffectiveNodelistEchoTag returns NodelistEchoTag if configured, otherwise
// DefaultNodelistEchoTag.
func (n *NetworkDef) EffectiveNodelistEchoTag() string {
	if n.NodelistEchoTag != "" {
		return n.NodelistEchoTag
	}
	return DefaultNodelistEchoTag
}

// IsHub reports whether this BBS is the authoritative hub of this network
// (no uplink configured) rather than a leaf/downlink polling an uplink.
func (n *NetworkDef) IsHub() bool {
	return n.Uplink == ""
}

// DownlinkByAddr finds a configured Downlink by address (ignoring point),
// or nil if addr isn't a known downlink for this network.
func (n *NetworkDef) DownlinkByAddr(addr Addr) *Downlink {
	for i := range n.Downlinks {
		a, err := ParseAddr(n.Downlinks[i].Address)
		if err != nil {
			continue
		}
		if a.Zone == addr.Zone && a.Net == addr.Net && a.Node == addr.Node {
			return &n.Downlinks[i]
		}
	}
	return nil
}
