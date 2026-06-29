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
//   v0.13.0 2026-06-27  VirtNet: find-or-create helpers for the conferences
//                        and file area this feature auto-creates the first
//                        time this BBS has a registered VirtNet address.
//                        No conference/file-area auto-creation existed
//                        anywhere in the codebase before this — scan.go/
//                        toss.go both skip unmapped areas rather than
//                        creating them — so this is genuinely new, narrowly
//                        scoped behavior, not a general mechanism.
// ============================================================================

// Package fido — ensure.go
//
// Note: only conference helpers live here, since internal/conferences has
// no problematic imports. internal/files, by contrast, transitively
// imports internal/config (for its own scan scheduling), and
// internal/config imports internal/fido — so internal/fido importing
// internal/files directly would be an import cycle. File-area access
// (NodeChgs.txt, registering the generated nodelist/diagram files) instead
// goes through the FileArea interface below, satisfied implicitly by
// *files.Store without fido ever importing that package — the standard Go
// way to break this kind of cycle.
package fido

import (
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
)

// FileArea is the minimal subset of *files.Store the VirtNet file-area
// features and TIC processor need. *files.Store satisfies this implicitly.
type FileArea interface {
	EnsureDir(name, description string) (dirID int64, dirPath string, err error)
	RegisterUpload(dirID int64, filename, description, uploader string) error
	UploadDir(dirID int64) string
	InstallFile(dirID int64, srcPath, destName, description, uploader string) error
	ListAreaFiles(dirID int64) ([]AreaFile, error)
}

// AreaFile is one registered file in a BBS file directory.
type AreaFile struct {
	Filename string
	FullPath string
	ModTime  time.Time
	Uploader string
}

// EnsureConference finds a conference literally named name, creating it
// with sane defaults if it doesn't exist yet. Used for "<NetworkName>
// Nodelists" and "<NetworkName> Sysops".
func EnsureConference(confStore *conferences.Store, name, network string) (*conferences.Conference, error) {
	c, err := confStore.GetByName(name)
	if err != nil {
		return nil, err
	}
	if c != nil {
		return c, nil
	}
	c = &conferences.Conference{
		Name:        name,
		Description: name + " (auto-created)",
		Public:      true,
		ReadSec:     0,
		WriteSec:    0,
		SysopSec:    100,
		Network:     network,
	}
	if err := confStore.Create(c); err != nil {
		return nil, err
	}
	return c, nil
}

// EnsureEchoConference is EnsureConference, additionally marking the
// conference as an echomail area under tag and wiring nd.Areas[tag] to its
// ID — used for the auto-distributed "<NetworkName> Nodelists" echo area
// (see nodelistecho.go). The caller is responsible for persisting nd's
// updated Areas map back to VirtBBS.DAT (internal/fido cannot save config
// itself — see members.go's ApproveJoinRequest comment for why).
func EnsureEchoConference(confStore *conferences.Store, name, network, tag string) (*conferences.Conference, error) {
	c, err := confStore.GetByName(name)
	if err != nil {
		return nil, err
	}
	if c != nil {
		if !c.Echo || c.EchoTag != tag {
			c.Echo = true
			c.EchoTag = tag
			if c.EchoFromName == "" {
				c.EchoFromName = conferences.EchoFromReal
			}
			if err := confStore.Update(c); err != nil {
				return nil, err
			}
		}
		return c, nil
	}
	c = &conferences.Conference{
		Name:         name,
		Description:  name + " (auto-created echomail)",
		Public:       true,
		ReadSec:      0,
		WriteSec:     0,
		SysopSec:     100,
		Echo:         true,
		EchoTag:      tag,
		EchoFromName: conferences.EchoFromReal,
		Network:      network,
	}
	if err := confStore.Create(c); err != nil {
		return nil, err
	}
	return c, nil
}

