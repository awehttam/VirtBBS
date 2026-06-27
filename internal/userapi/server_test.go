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
//   v0.6.0  2026-06-26  Phase 0 (VirtAnd/VirtTerm): smoke test against a temp
//                        SQLite DB — token login, security-filtered conference
//                        and file listing, nodelist-version check.
// ============================================================================

package userapi

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/users"
)

func TestUserAPISmoke(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	filesRoot := filepath.Join(dir, "files")
	_ = os.MkdirAll(filesRoot, 0755)

	sqlDB, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer sqlDB.Close()

	userStore, err := users.Open(sqlDB)
	if err != nil {
		t.Fatalf("users.Open: %v", err)
	}

	msgStore, err := messages.Open(sqlDB)
	if err != nil {
		t.Fatalf("messages.Open: %v", err)
	}

	confStore, err := conferences.Open(sqlDB)
	if err != nil {
		t.Fatalf("conferences.Open: %v", err)
	}

	fileStore, err := files.Open(sqlDB, filesRoot)
	if err != nil {
		t.Fatalf("files.Open: %v", err)
	}

	// Create two conferences with different ReadSec, and one regular user.
	if err := confStore.Create(&conferences.Conference{ID: 1, Name: "Public", ReadSec: 10}); err != nil {
		t.Fatalf("create conf 1: %v", err)
	}
	if err := confStore.Create(&conferences.Conference{ID: 2, Name: "SysopOnly", ReadSec: 100}); err != nil {
		t.Fatalf("create conf 2: %v", err)
	}

	u := &users.User{Name: "TestUser", SecurityLevel: 20, PageLength: 24, XferProtocol: "Z", ANSI: true}
	if err := userStore.Create(u, "password123"); err != nil {
		t.Fatalf("create user: %v", err)
	}

	rawToken, err := userStore.CreateAPIToken(u.ID, "test-device")
	if err != nil {
		t.Fatalf("CreateAPIToken: %v", err)
	}

	// Record a nodelist version so fido.nodelist.version has something to return.
	if err := fido.RecordNodelistVersion(msgStore.DB(), "FidoNet", 42); err != nil {
		t.Fatalf("RecordNodelistVersion: %v", err)
	}

	srv := &Server{
		Addr: "127.0.0.1:0",
		Deps: Deps{Users: userStore, Messages: msgStore, Conferences: confStore, Files: fileStore},
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go srv.handle(c)
		}
	}()
	defer ln.Close()

	addr := ln.Addr().String()

	call := func(method string, params any, auth AuthParams) Response {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		defer conn.Close()
		paramsJSON, _ := json.Marshal(params)
		req := Request{Method: method, Params: paramsJSON, Auth: auth}
		reqJSON, _ := json.Marshal(req)
		if _, err := conn.Write(append(reqJSON, '\n')); err != nil {
			t.Fatalf("write: %v", err)
		}
		sc := bufio.NewScanner(conn)
		if !sc.Scan() {
			t.Fatalf("no response for %s: %v", method, sc.Err())
		}
		var resp Response
		if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal response for %s: %v", method, err)
		}
		return resp
	}

	// Bad token → unauthorized.
	if resp := call("conferences.list", nil, AuthParams{Token: "bogus"}); resp.Error != "unauthorized" {
		t.Fatalf("expected unauthorized, got %+v", resp)
	}

	// Good token, security-filtered conference list: the default "General"
	// (ReadSec 10) plus our "Public" (ReadSec 10) should appear for a
	// SecurityLevel=20 user, but "SysopOnly" (ReadSec 100) should not.
	resp := call("conferences.list", nil, AuthParams{Token: rawToken})
	if resp.Error != "" {
		t.Fatalf("conferences.list error: %s", resp.Error)
	}
	list, ok := resp.Result.([]any)
	if !ok || len(list) != 2 {
		t.Fatalf("expected 2 visible conferences, got %+v", resp.Result)
	}

	// files.dirs.list — should succeed (empty list) with no error.
	if resp := call("files.dirs.list", nil, AuthParams{Token: rawToken}); resp.Error != "" {
		t.Fatalf("files.dirs.list error: %s", resp.Error)
	}

	// fido.nodelist.version — should return the recorded version.
	resp = call("fido.nodelist.version", map[string]string{"network": "FidoNet"}, AuthParams{Token: rawToken})
	if resp.Error != "" {
		t.Fatalf("fido.nodelist.version error: %s", resp.Error)
	}
	m, ok := resp.Result.(map[string]any)
	if !ok || m["network"] != "FidoNet" || m["node_count"].(float64) != 42 {
		t.Fatalf("unexpected nodelist version result: %+v", resp.Result)
	}

	// Revoke the token, confirm it no longer authenticates.
	tokens, err := userStore.ListAPITokens(u.ID)
	if err != nil || len(tokens) != 1 {
		t.Fatalf("ListAPITokens: %v / %+v", err, tokens)
	}
	if err := userStore.RevokeAPIToken(u.ID, tokens[0].ID); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if resp := call("conferences.list", nil, AuthParams{Token: rawToken}); resp.Error != "unauthorized" {
		t.Fatalf("expected unauthorized after revoke, got %+v", resp)
	}
}
