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
//   v0.0.1  2026-06-24  Initial implementation
//   v0.0.5  2026-06-24  Phase 12/14: added Doors []door.Config and CallerLogPath
//   v0.6.0  2026-06-26  Phase 0 (VirtAnd/VirtTerm): NetworkConfig.UserAPIPort/UserAPIBind
//                        for the new token-authenticated internal/userapi listener
//   v0.13.0 2026-06-28  Web UI: NetworkConfig.WebPort/WebBind, PathsConfig.WWW
// ============================================================================

// Package config manages VirtBBS.DAT — the TOML configuration file.
package config

import (
	"os"
	"sync"

	"github.com/BurntSushi/toml"

	"github.com/virtbbs/virtbbs/internal/door"
	"github.com/virtbbs/virtbbs/internal/fido"
)

// Config holds all VirtBBS runtime configuration.
// json tags mirror the toml tags so the API returns clean lowercase keys.
type Config struct {
	Network NetworkConfig `toml:"network" json:"network"`
	Paths   PathsConfig   `toml:"paths"   json:"paths"`
	Sysop   SysopConfig   `toml:"sysop"   json:"sysop"`
	BBS     BBSConfig     `toml:"bbs"     json:"bbs"`
	Session SessionConfig `toml:"session" json:"session"`
	Fido    fido.Config   `toml:"fido"    json:"fido"`
	Doors   []door.Config `toml:"doors"   json:"doors"`
}

// SessionConfig controls per-caller session limits and associated paths.
type SessionConfig struct {
	IdleTimeoutMins int    `toml:"idle_timeout_mins"  json:"idle_timeout_mins"`
	TimePerCallMins int    `toml:"time_per_call_mins" json:"time_per_call_mins"`
	DisplayDir      string `toml:"display_dir"        json:"display_dir"`
	NewUserSecurity int    `toml:"new_user_security"  json:"new_user_security"`
}

type NetworkConfig struct {
	TelnetPort   int    `toml:"telnet_port"    json:"telnet_port"`
	SSHPort      int    `toml:"ssh_port"       json:"ssh_port"`
	APIPort      int    `toml:"api_port"       json:"api_port"`
	APIBind      string `toml:"api_bind"       json:"api_bind"`
	UserAPIPort  int    `toml:"userapi_port"   json:"userapi_port"` // VirtAnd token-authenticated API
	UserAPIBind  string `toml:"userapi_bind"   json:"userapi_bind"`
	WebPort      int    `toml:"web_port"       json:"web_port"`      // browser-based BBS UI (internal/web)
	WebBind      string `toml:"web_bind"       json:"web_bind"`
}

type PathsConfig struct {
	DB        string `toml:"db"         json:"db"`
	Files     string `toml:"files"      json:"files"`
	Logs      string `toml:"logs"       json:"logs"`
	CallerLog string `toml:"caller_log" json:"caller_log"`
	WWW       string `toml:"www"        json:"www"` // web UI templates and static files
}

type SysopConfig struct {
	Name         string `toml:"name"     json:"name"`
	PasswordHash string `toml:"password" json:"password,omitempty"`
}

type BBSConfig struct {
	Name     string `toml:"name"      json:"name"`
	MaxNodes int    `toml:"max_nodes" json:"max_nodes"`
}

var (
	mu       sync.RWMutex
	current  *Config
	filepath string
)

// Load reads VirtBBS.DAT from path, caches it, and returns it.
func Load(path string) (*Config, error) {
	cfg := defaults()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// File doesn't exist yet — use defaults.
	}
	mu.Lock()
	current = cfg
	filepath = path
	mu.Unlock()
	return cfg, nil
}

// Get returns the currently loaded config (read-only). Panics if Load not called.
func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Save writes cfg to VirtBBS.DAT and updates the in-memory cache so that
// Get() reflects the change immediately for all active sessions.
func Save(cfg *Config) error {
	mu.Lock()
	defer mu.Unlock()
	f, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	if err := enc.Encode(cfg); err != nil {
		return err
	}
	current = cfg
	return nil
}

func defaults() *Config {
	return &Config{
		Network: NetworkConfig{
			TelnetPort:   2323,
			SSHPort:      3232,
			APIPort:      9999,
			APIBind:      "0.0.0.0",
			UserAPIPort:  9998,
			UserAPIBind:  "0.0.0.0",
			WebPort:      8081,
			WebBind:      "0.0.0.0",
		},
		Paths: PathsConfig{
			DB:        "./data/virtbbs.db",
			Files:     "./files",
			Logs:      "./logs",
			CallerLog: "./logs/CALLERS.LOG",
			WWW:       "www",
		},
		Sysop: SysopConfig{
			Name: "Sysop",
		},
		BBS: BBSConfig{
			Name:     "VirtBBS",
			MaxNodes: 10,
		},
		Session: SessionConfig{
			IdleTimeoutMins: 5,
			TimePerCallMins: 60,
			DisplayDir:      "display",
			NewUserSecurity: 10,
		},
		Fido: fido.DefaultConfig(),
	}
}
