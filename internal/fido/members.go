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
//   v0.13.0 2026-06-27  VirtNet: join requests + approved members (the
//                        network's routing table and nodelist source of
//                        truth) for networks this BBS hosts (Uplink=="").
// ============================================================================

// Package fido — members.go
//
// Join-request queue and approved-member registry for a network this BBS
// hosts as its hub (NetworkDef.IsHub() == true). A Member row is both an
// entry in the routing table (binkp_host/password) and a row the nodelist
// generator (nodelistgen.go) reads directly.
package fido

import (
	"database/sql"
	"fmt"
	"time"
)

// JoinRequest is a pending (or decided) application to join a hub network.
type JoinRequest struct {
	ID                int64  `json:"ID"`
	Network           string `json:"Network"`
	RequestedByUserID int    `json:"RequestedByUserID"`
	BBSName           string `json:"BBSName"`
	SysopName         string `json:"SysopName"`
	Location          string `json:"Location"`
	Contact           string `json:"Contact"`
	RequestedNet      *int   `json:"RequestedNet,omitempty"`
	BinkpHost         string `json:"BinkpHost"`
	Status            string `json:"Status"`
	CreatedAt         string `json:"CreatedAt"`
	DecidedAt         string `json:"DecidedAt,omitempty"`
	DecidedBy         string `json:"DecidedBy,omitempty"`
}

// Member is one approved (or remotely-announced) node of a hub network.
type Member struct {
	ID          int64  `json:"ID"`
	Network     string `json:"Network"`
	Zone        int    `json:"Zone"`
	Net         int    `json:"Net"`
	NodeNum     int    `json:"NodeNum"`
	Point       int    `json:"Point"`
	BBSName     string `json:"BBSName"`
	SysopName   string `json:"SysopName"`
	Location    string `json:"Location"`
	Contact     string `json:"Contact"`
	BinkpHost   string `json:"BinkpHost"`
	Password    string `json:"Password"`
	IsHost      bool   `json:"IsHost"`
	IsActive    bool   `json:"IsActive"`
	IsDelegated bool   `json:"IsDelegated"`
	JoinedAt    string `json:"JoinedAt"`
}

// Addr returns the member's address as an Addr.
func (m *Member) Addr() Addr {
	return Addr{Zone: m.Zone, Net: m.Net, Node: m.NodeNum, Point: m.Point}
}

// Addr4D returns the member's 4D address string.
func (m *Member) Addr4D() string {
	return m.Addr().String()
}

// MembersDB wraps the shared database for join-request/member operations.
type MembersDB struct{ db *sql.DB }

// OpenMembersDB returns a MembersDB using the shared database connection.
func OpenMembersDB(db *sql.DB) *MembersDB { return &MembersDB{db: db} }

// SubmitJoinRequest queues a new application to join network.
func (mdb *MembersDB) SubmitJoinRequest(req *JoinRequest) (int64, error) {
	res, err := mdb.db.Exec(`INSERT INTO fido_join_requests
		(network, requested_by_user_id, bbs_name, sysop_name, location, contact, requested_net, binkp_host, status, created_at)
		VALUES (?,?,?,?,?,?,?,?,'pending',?)`,
		req.Network, req.RequestedByUserID, req.BBSName, req.SysopName,
		req.Location, req.Contact, req.RequestedNet, req.BinkpHost,
		time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListPending returns all pending join requests for network.
func (mdb *MembersDB) ListPending(network string) ([]*JoinRequest, error) {
	rows, err := mdb.db.Query(`SELECT id, network, requested_by_user_id, bbs_name, sysop_name,
		location, contact, requested_net, binkp_host, status, created_at
		FROM fido_join_requests WHERE network=? AND status='pending' ORDER BY id`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*JoinRequest
	for rows.Next() {
		r := &JoinRequest{}
		if err := rows.Scan(&r.ID, &r.Network, &r.RequestedByUserID, &r.BBSName, &r.SysopName,
			&r.Location, &r.Contact, &r.RequestedNet, &r.BinkpHost, &r.Status, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetJoinRequest fetches one join request by ID.
func (mdb *MembersDB) GetJoinRequest(id int64) (*JoinRequest, error) {
	r := &JoinRequest{}
	err := mdb.db.QueryRow(`SELECT id, network, requested_by_user_id, bbs_name, sysop_name,
		location, contact, requested_net, binkp_host, status, created_at
		FROM fido_join_requests WHERE id=?`, id).
		Scan(&r.ID, &r.Network, &r.RequestedByUserID, &r.BBSName, &r.SysopName,
			&r.Location, &r.Contact, &r.RequestedNet, &r.BinkpHost, &r.Status, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Deny marks a join request as denied.
func (mdb *MembersDB) Deny(id int64, decidedBy string) error {
	_, err := mdb.db.Exec(`UPDATE fido_join_requests SET status='denied', decided_at=?, decided_by=? WHERE id=?`,
		time.Now().Format(time.RFC3339), decidedBy, id)
	return err
}

// NextNodeNum returns the next available node number in network/net,
// reusing this codebase's existing SELECT COALESCE(MAX(...),0)+1 ID
// allocation idiom (see messages.Store.Post).
func (mdb *MembersDB) NextNodeNum(network string, net int) (int, error) {
	var next int
	err := mdb.db.QueryRow(`SELECT COALESCE(MAX(node_num),0)+1 FROM fido_members
		WHERE network=? AND net=? AND point=0`, network, net).Scan(&next)
	return next, err
}

// NetHasMembers reports whether network/net already has any members —
// used by the approval UI to detect "first member of a new net" and offer
// marking them as that net's Host.
func (mdb *MembersDB) NetHasMembers(network string, net int) (bool, error) {
	var n int
	err := mdb.db.QueryRow(`SELECT COUNT(*) FROM fido_members WHERE network=? AND net=?`, network, net).Scan(&n)
	return n > 0, err
}

// InsertMember inserts a brand-new member row.
func (mdb *MembersDB) InsertMember(m *Member) (int64, error) {
	isHost, isActive, isDelegated := boolToInt(m.IsHost), boolToInt(m.IsActive), boolToInt(m.IsDelegated)
	res, err := mdb.db.Exec(`INSERT INTO fido_members
		(network, zone, net, node_num, point, bbs_name, sysop_name, location, contact, binkp_host, password, is_host, is_active, is_delegated, joined_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.Network, m.Zone, m.Net, m.NodeNum, m.Point, m.BBSName, m.SysopName,
		m.Location, m.Contact, m.BinkpHost, m.Password, isHost, isActive, isDelegated,
		time.Now().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpsertMember inserts a member row, or updates the existing one for the
// same (network, zone, net, node_num, point) if it already exists. Returns
// the resulting member and whether it was a brand-new insert (true) or an
// update to an existing row (false). Used by ProcessNodeAnnounce, since an
// inbound announcement doesn't know whether the address is new or already
// known locally.
func (mdb *MembersDB) UpsertMember(m *Member) (*Member, bool, error) {
	existing, err := mdb.GetMemberByAddr(m.Network, m.Addr())
	if err != nil {
		return nil, false, err
	}
	if existing == nil {
		id, err := mdb.InsertMember(m)
		if err != nil {
			return nil, false, err
		}
		m.ID = id
		return m, true, nil
	}
	m.ID = existing.ID
	m.IsHost = existing.IsHost
	if err := mdb.UpdateMemberInfo(m); err != nil {
		return nil, false, err
	}
	return m, false, nil
}

// UpdateMemberInfo updates a member's contact/location/binkp/password info
// in place, keyed by ID.
func (mdb *MembersDB) UpdateMemberInfo(m *Member) error {
	_, err := mdb.db.Exec(`UPDATE fido_members SET
		bbs_name=?, sysop_name=?, location=?, contact=?, binkp_host=?, password=?
		WHERE id=?`,
		m.BBSName, m.SysopName, m.Location, m.Contact, m.BinkpHost, m.Password, m.ID)
	return err
}

// GetMemberByAddr finds a member by its exact address, or nil if none.
func (mdb *MembersDB) GetMemberByAddr(network string, a Addr) (*Member, error) {
	m, err := scanMember(mdb.db.QueryRow(`SELECT `+memberCols+` FROM fido_members
		WHERE network=? AND zone=? AND net=? AND node_num=? AND point=?`,
		network, a.Zone, a.Net, a.Node, a.Point))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

// ListMembers returns every member of network, ordered by net/node — the
// routing table / nodelist source of truth.
func (mdb *MembersDB) ListMembers(network string) ([]*Member, error) {
	rows, err := mdb.db.Query(`SELECT `+memberCols+` FROM fido_members WHERE network=? ORDER BY net, node_num`, network)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Member
	for rows.Next() {
		m, err := scanMemberRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

const memberCols = `id, network, zone, net, node_num, point, bbs_name, sysop_name,
	location, contact, binkp_host, password, is_host, is_active, is_delegated, joined_at`

type rowOrRows interface {
	Scan(...any) error
}

func scanMember(row rowOrRows) (*Member, error) {
	return scanMemberRow(row)
}

func scanMemberRow(row rowOrRows) (*Member, error) {
	m := &Member{}
	var isHost, isActive, isDelegated int
	err := row.Scan(&m.ID, &m.Network, &m.Zone, &m.Net, &m.NodeNum, &m.Point,
		&m.BBSName, &m.SysopName, &m.Location, &m.Contact, &m.BinkpHost, &m.Password,
		&isHost, &isActive, &isDelegated, &m.JoinedAt)
	if err != nil {
		return nil, err
	}
	m.IsHost = isHost != 0
	m.IsActive = isActive != 0
	m.IsDelegated = isDelegated != 0
	return m, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ApproveJoinRequest approves req, allocating it net/node (or using the
// caller-supplied node, e.g. 0 for a net's Host), and returns the new
// Member. saveDownlink is called to authorize the new member to poll
// BinkP immediately — internal/fido cannot persist VirtBBS.DAT itself
// (internal/config already imports internal/fido, so the reverse import
// would cycle), so the caller (internal/session) supplies a closure that
// wraps config.Get()/config.Save() exactly like the existing
// updateNetworkDownlinks helper already does for AreaFix downlinks.
func (mdb *MembersDB) ApproveJoinRequest(nd *NetworkDef, req *JoinRequest, net, node int, isHost bool, password string,
	saveDownlink func(networkName string, dl Downlink) error) (*Member, error) {
	our := nd.NodeAddr()
	if our == (Addr{}) {
		return nil, fmt.Errorf("invalid network address %q", nd.Address)
	}

	m := &Member{
		Network:   nd.Name,
		Zone:      our.Zone,
		Net:       net,
		NodeNum:   node,
		BBSName:   req.BBSName,
		SysopName: req.SysopName,
		Location:  req.Location,
		Contact:   req.Contact,
		BinkpHost: req.BinkpHost,
		Password:  password,
		IsHost:    isHost,
		IsActive:  true,
	}
	id, err := mdb.InsertMember(m)
	if err != nil {
		return nil, err
	}
	m.ID = id

	if saveDownlink != nil {
		dl := Downlink{Name: m.BBSName, Address: m.Addr4D(), Password: password}
		if isHost {
			// A net's Host is, by the standard BinkleyTerm/FrontDoor
			// convention, also addressable at zone:net/0 — see
			// NetworkDef.AllAddrs' doc comment for the full rationale.
			dl.AKAs = []string{fmt.Sprintf("%d:%d/0", m.Zone, m.Net)}
		}
		if err := saveDownlink(nd.Name, dl); err != nil {
			return m, fmt.Errorf("member created but failed to authorize BinkP: %w", err)
		}
	}

	if isHost {
		// "The default routing of hubs": any net's Host automatically
		// gets a ROUTES.BBS-style default route the moment it's created.
		if err := SeedDefaultHubRoute(mdb.db, nd.Name, m.Zone, m.Net); err != nil {
			return m, fmt.Errorf("member created but failed to seed default route: %w", err)
		}
	}

	// Auto-subscribe to nodelist updates — the only "no opt-out" AreaFix
	// subscription in the system; every other echo area still requires an
	// explicit AreaFix +TAG request.
	if err := OpenAreaFixDB(mdb.db).Subscribe(nd.Name, m.Addr4D(), nd.EffectiveNodelistEchoTag()); err != nil {
		return m, fmt.Errorf("member created but failed to subscribe to nodelist updates: %w", err)
	}

	_, err = mdb.db.Exec(`UPDATE fido_join_requests SET status='approved', decided_at=? WHERE id=?`,
		time.Now().Format(time.RFC3339), req.ID)
	return m, err
}
