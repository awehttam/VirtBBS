package fido

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// LocalNodeInput is one node row submitted by the sysop GUI editor.
type LocalNodeInput struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	Location string `json:"location"`
	Sysop    string `json:"sysop"`
	Phone    string `json:"phone"`
	Baud     int    `json:"baud"`
	Flags    string `json:"flags"`
	Type     string `json:"type"`
	Active   bool   `json:"active"`
}

// LocalNodelistCommitParams batches pending editor changes.
type LocalNodelistCommitParams struct {
	Network string           `json:"network"`
	Upsert  []LocalNodeInput `json:"upsert"`
	Delete  []string         `json:"delete"` // FTN addresses to remove
}

// LocalNodelistCommitResult summarises a commit operation.
type LocalNodelistCommitResult struct {
	NodeCount    int    `json:"node_count"`
	NodelistFile string `json:"nodelist_file"`
	NodediffFile string `json:"nodediff_file"`
	NetmailSent  bool   `json:"netmail_sent"`
	NetmailTo    string `json:"netmail_to,omitempty"`
	Message      string `json:"message,omitempty"`
}

// EnsureOwnNode upserts this BBS's own address (and configured AKAs) into the local nodelist DB.
func EnsureOwnNode(db *sql.DB, nd *NetworkDef, bbsName, sysopName, location string, telnetPort int) error {
	return RestoreLocalNodeEntries(db, nd, bbsName, sysopName, location, telnetPort)
}

// RestoreLocalNodeEntries re-applies this BBS's configured addresses (primary
// plus AKAs from NetworkDef.AllAddrs) into fido_nodes. Call after an external
// nodelist import replaces the table, or when AKAs are added to config.
func RestoreLocalNodeEntries(db *sql.DB, nd *NetworkDef, bbsName, sysopName, location string, telnetPort int) error {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return nil
	}
	if bbsName == "" {
		bbsName = "VirtBBS"
	}
	if sysopName == "" {
		sysopName = "Sysop"
	}
	if location == "" {
		location = "Internet"
	}

	ndb := OpenNodelistDB(db)
	for _, addr := range nd.AllAddrs() {
		e := localNodeEntryForAddr(nd, addr, bbsName, sysopName, location, telnetPort)
		if err := ndb.UpsertLocalNode(e); err != nil {
			return err
		}
	}
	count, err := ndb.Count(nd.Name)
	if err != nil {
		return err
	}
	return bumpNodelistVersionCount(db, nd.Name, count)
}

func localNodeEntryForAddr(nd *NetworkDef, addr Addr, bbsName, sysopName, location string, telnetPort int) *NodeEntry {
	flags := nd.NodeFlags
	if len(flags) == 0 {
		flags = DefaultNodeFlags
	}
	flagsStr := BuildNodelistFlags(flags, nd.BinkpHost, nd.Port(), telnetPort)
	nodeType := "Node"
	if addr.Node == 0 && addr.Point == 0 {
		nodeType = "Host"
	}
	return &NodeEntry{
		Network:  nd.Name,
		Zone:     addr.Zone,
		Net:      addr.Net,
		Node:     addr.Node,
		Point:    addr.Point,
		Name:     bbsName,
		Location: location,
		Sysop:    sysopName,
		Phone:    "-Unpublished-",
		Baud:     33600,
		Flags:    flagsStr,
		Type:     nodeType,
		Active:   true,
	}
}

// EnsureAllNetworkOwnNodes ensures every enabled network has its own node row.
func EnsureAllNetworkOwnNodes(db *sql.DB, networks []NetworkDef, bbsName, sysopName string, telnetPort int) {
	for _, nd := range networks {
		if !nd.Enabled {
			continue
		}
		n := nd
		_ = EnsureOwnNode(db, &n, bbsName, sysopName, "Internet", telnetPort)
	}
}

// ListLocalNodes returns every node in the local nodelist for a network.
func ListLocalNodes(db *sql.DB, network string) ([]NodeEntry, error) {
	return OpenNodelistDB(db).ListAll(network)
}

// ApplyLocalNodes upserts and deletes rows without writing nodelist files.
func ApplyLocalNodes(db *sql.DB, network string, upsert []LocalNodeInput, delete []string) error {
	ndb := OpenNodelistDB(db)
	for _, addrStr := range delete {
		a, err := ParseAddr(strings.TrimSpace(addrStr))
		if err != nil {
			return fmt.Errorf("delete address %q: %w", addrStr, err)
		}
		if err := ndb.DeleteAddr(network, a); err != nil {
			return err
		}
	}
	for _, in := range upsert {
		e, err := localInputToEntry(network, in)
		if err != nil {
			return err
		}
		if err := ndb.UpsertLocalNode(e); err != nil {
			return err
		}
	}
	return nil
}

// CommitLocalNodelist applies pending changes, writes NODELIST + NODEDIFF files,
// and queues netmail to the uplink when configured.
func CommitLocalNodelist(db *sql.DB, nd *NetworkDef, bbsName, sysopName string, telnetPort int, p LocalNodelistCommitParams) (*LocalNodelistCommitResult, error) {
	if err := ApplyLocalNodes(db, nd.Name, p.Upsert, p.Delete); err != nil {
		return nil, err
	}
	_ = EnsureOwnNode(db, nd, bbsName, sysopName, "Internet", telnetPort)

	ndb := OpenNodelistDB(db)
	before, _ := ndb.ListAll(nd.Name)
	beforeMap := map[string]*NodeEntry{}
	for i := range before {
		beforeMap[nodeKey(&before[i])] = &before[i]
	}

	nodelistPath, nodelistBody, err := ExportNodelistFile(db, nd)
	if err != nil {
		return nil, err
	}

	after, err := ndb.ListAll(nd.Name)
	if err != nil {
		return nil, err
	}

	var diffLines []string
	for i := range after {
		key := nodeKey(&after[i])
		old := beforeMap[key]
		if old == nil || nodeEntryChanged(old, &after[i]) {
			diffLines = append(diffLines, encodeNodediffLine(&after[i]))
		}
	}
	// Nodes removed entirely.
	afterMap := map[string]bool{}
	for i := range after {
		afterMap[nodeKey(&after[i])] = true
	}
	for i := range before {
		if !afterMap[nodeKey(&before[i])] {
			diffLines = append(diffLines, "; removed "+before[i].Addr4D())
		}
	}

	diffFile := NodelistDiffFilename(time.Now())
	var diffBody []byte
	if len(diffLines) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, ";NODEDIFF for %s, generated %s\r\n", nd.Name, time.Now().Format(time.RFC3339))
		for _, line := range diffLines {
			fmt.Fprintf(&b, "%s\r\n", line)
		}
		diffBody = []byte(b.String())
		if nd.NodelistDir != "" {
			_ = os.MkdirAll(nd.NodelistDir, 0755)
			_ = os.WriteFile(filepath.Join(nd.NodelistDir, diffFile), diffBody, 0644)
		}
	}

	result := &LocalNodelistCommitResult{
		NodeCount:    len(after),
		NodelistFile: nodelistPath,
		NodediffFile: diffFile,
	}

	if len(diffBody) > 0 && nd.UplinkAddr() != (Addr{}) {
		pktPath, toAddr, err := sendNodediffNetmail(nd, nd.NodeAddr(), diffBody, diffFile)
		if err != nil {
			result.Message = "nodelist saved; netmail failed: " + err.Error()
		} else {
			result.NetmailSent = true
			result.NetmailTo = toAddr
			result.Message = fmt.Sprintf("nodelist committed (%d nodes); NODEDIFF sent to %s", len(after), toAddr)
			_ = pktPath
		}
	} else if len(diffBody) == 0 {
		result.Message = fmt.Sprintf("nodelist committed (%d nodes); no changes for NODEDIFF", len(after))
	} else {
		result.Message = fmt.Sprintf("nodelist committed (%d nodes); NODEDIFF saved locally (no uplink)", len(after))
	}
	_ = nodelistBody
	return result, nil
}

// ExportNodelistFile writes the full nodelist for network nd to nodelist_dir.
func ExportNodelistFile(db *sql.DB, nd *NetworkDef) (path string, body []byte, err error) {
	nodes, err := OpenNodelistDB(db).ListAll(nd.Name)
	if err != nil {
		return "", nil, err
	}
	body = EncodeNodelistBytes(nd.Name, nodes)
	filename := NodelistFullFilename(time.Now())
	if !IsWeeklyNodelistDay(time.Now()) || nd.NodelistDir == "" {
		return filename, body, nil
	}
	if err := os.MkdirAll(nd.NodelistDir, 0755); err != nil {
		return "", nil, err
	}
	path = filepath.Join(nd.NodelistDir, filename)
	if err := os.WriteFile(path, body, 0644); err != nil {
		return "", nil, err
	}
	return path, body, nil
}

// EncodeNodelistBytes builds a FTS-0005 nodelist file from node rows.
func EncodeNodelistBytes(network string, nodes []NodeEntry) []byte {
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Zone != nodes[j].Zone {
			return nodes[i].Zone < nodes[j].Zone
		}
		if nodes[i].Net != nodes[j].Net {
			return nodes[i].Net < nodes[j].Net
		}
		if nodes[i].Node != nodes[j].Node {
			return nodes[i].Node < nodes[j].Node
		}
		return nodes[i].Point < nodes[j].Point
	})

	var b strings.Builder
	fmt.Fprintf(&b, ";VirtBBS local nodelist for %q, generated %s\r\n", network, time.Now().Format(time.RFC3339))
	for i := range nodes {
		fmt.Fprintf(&b, "%s\r\n", encodeNodelistLine(&nodes[i]))
	}
	return []byte(b.String())
}

func encodeNodelistLine(e *NodeEntry) string {
	keyword := nodelistKeyword(e.Type)
	num := e.Node
	switch strings.ToLower(e.Type) {
	case "zone":
		num = e.Zone
	case "region", "host":
		num = e.Net
	case "hub", "pvt", "hold", "down", "boss", "node", "":
		num = e.Node
	}
	if keyword == "" {
		keyword = ""
		num = e.Node
	}
	phone := e.Phone
	if phone == "" {
		phone = "-Unpublished-"
	}
	baud := e.Baud
	if baud == 0 {
		baud = 33600
	}
	if keyword == "" {
		return fmt.Sprintf(",%d,%s,%s,%s,%s,%d,%s",
			num, nlEncode(e.Name), nlEncode(e.Location), nlEncode(e.Sysop),
			phone, baud, e.Flags)
	}
	return fmt.Sprintf("%s,%d,%s,%s,%s,%s,%d,%s",
		keyword, num, nlEncode(e.Name), nlEncode(e.Location), nlEncode(e.Sysop),
		phone, baud, e.Flags)
}

func encodeNodediffLine(e *NodeEntry) string {
	phone := e.Phone
	if phone == "" {
		phone = "-Unpublished-"
	}
	baud := e.Baud
	if baud == 0 {
		baud = 33600
	}
	return fmt.Sprintf(",%d,%s,%s,%s,%s,%d,%s",
		e.Node, nlEncode(e.Name), nlEncode(e.Location), nlEncode(e.Sysop),
		phone, baud, e.Flags)
}

func nodelistKeyword(nodeType string) string {
	switch strings.ToLower(strings.TrimSpace(nodeType)) {
	case "zone":
		return "Zone"
	case "region":
		return "Region"
	case "host":
		return "Host"
	case "hub":
		return "Hub"
	case "pvt":
		return "Pvt"
	case "hold":
		return "Hold"
	case "down":
		return "Down"
	case "boss":
		return "Boss"
	default:
		return ""
	}
}

func localInputToEntry(network string, in LocalNodeInput) (*NodeEntry, error) {
	a, err := ParseAddr(strings.TrimSpace(in.Address))
	if err != nil {
		return nil, fmt.Errorf("address %q: %w", in.Address, err)
	}
	nodeType := in.Type
	if nodeType == "" {
		nodeType = "Node"
	}
	phone := in.Phone
	if phone == "" {
		phone = "-Unpublished-"
	}
	baud := in.Baud
	if baud == 0 {
		baud = 33600
	}
	return &NodeEntry{
		Network:  network,
		Zone:     a.Zone,
		Net:      a.Net,
		Node:     a.Node,
		Point:    a.Point,
		Name:     in.Name,
		Location: in.Location,
		Sysop:    in.Sysop,
		Phone:    phone,
		Baud:     baud,
		Flags:    in.Flags,
		Type:     nodeType,
		Active:   in.Active,
	}, nil
}

func nodeKey(e *NodeEntry) string {
	return fmt.Sprintf("%d:%d/%d.%d", e.Zone, e.Net, e.Node, e.Point)
}
