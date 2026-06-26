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
//   v0.0.2  2026-06-24  In-memory node registry: kick, broadcast, chat
// ============================================================================

// Package node — registry.go: in-memory control handles for live sessions.
// The SQLite node table tracks persistent status for display; this registry
// provides live channels so the API (and other sessions) can push messages to
// a node or forcibly close its connection.
package node

import (
	"fmt"
	"sync"
)

// NodeControl is the in-memory control handle for one active session.
type NodeControl struct {
	// Messages is a buffered channel of text to display on this node's terminal.
	Messages chan string

	done      chan struct{} // closed when the session exits
	closeOnce sync.Once
	closeFn   func() // closes the underlying network connection
}

// SendMessage delivers text to this node's terminal (non-blocking).
// Returns false if the message buffer is full.
func (nc *NodeControl) SendMessage(msg string) bool {
	select {
	case nc.Messages <- msg:
		return true
	default:
		return false
	}
}

// Kick forcibly closes this node's network connection.
func (nc *NodeControl) Kick() {
	nc.closeOnce.Do(nc.closeFn)
}

// Done returns a channel that is closed when the session exits.
func (nc *NodeControl) Done() <-chan struct{} { return nc.done }

// Finish signals that the session has exited (closes Done and Messages).
func (nc *NodeControl) Finish() {
	close(nc.done)
}

// ── Global registry ───────────────────────────────────────────────────────────

var reg struct {
	mu   sync.RWMutex
	regs map[int]*NodeControl
}

func init() { reg.regs = make(map[int]*NodeControl) }

// RegisterControl adds a new live node to the registry.
// closeFn should close the underlying network connection.
func RegisterControl(nodeID int, closeFn func()) *NodeControl {
	nc := &NodeControl{
		Messages: make(chan string, 64),
		done:     make(chan struct{}),
		closeFn:  closeFn,
	}
	reg.mu.Lock()
	reg.regs[nodeID] = nc
	reg.mu.Unlock()
	return nc
}

// UnregisterControl removes a node from the registry.
func UnregisterControl(nodeID int) {
	reg.mu.Lock()
	delete(reg.regs, nodeID)
	reg.mu.Unlock()
}

// ActiveIDs returns the node IDs that currently have a live network connection.
func ActiveIDs() []int {
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	ids := make([]int, 0, len(reg.regs))
	for id := range reg.regs {
		ids = append(ids, id)
	}
	return ids
}

// KickNode forcibly disconnects the node with the given ID.
func KickNode(nodeID int) error {
	reg.mu.RLock()
	nc, ok := reg.regs[nodeID]
	reg.mu.RUnlock()
	if !ok {
		return fmt.Errorf("node %d is not active", nodeID)
	}
	nc.Kick()
	return nil
}

// BroadcastAll sends a highlighted message to every active node.
func BroadcastAll(from, msg string) {
	text := fmt.Sprintf("\r\n\033[1;33m*** %s broadcasts: %s ***\033[0m\r\n", from, msg)
	reg.mu.RLock()
	defer reg.mu.RUnlock()
	for _, nc := range reg.regs {
		nc.SendMessage(text)
	}
}

// ChatNode sends a personal message from one node/user to a specific node.
func ChatNode(toNodeID int, fromName, msg string) error {
	reg.mu.RLock()
	nc, ok := reg.regs[toNodeID]
	reg.mu.RUnlock()
	if !ok {
		return fmt.Errorf("node %d is not active", toNodeID)
	}
	text := fmt.Sprintf("\r\n\033[1;36m*** Chat from %s: %s ***\033[0m\r\n", fromName, msg)
	if !nc.SendMessage(text) {
		return fmt.Errorf("node %d message buffer full", toNodeID)
	}
	return nil
}
