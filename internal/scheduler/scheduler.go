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
//   v0.3.0  2026-06-25  Initial implementation — automatic per-network
//                        FidoNet poll+toss scheduler
//   v0.5.0  2026-06-25  Add a second per-network ticker for automatic
//                        nodelist fetching (fido.FetchAndImport)
// ============================================================================

// Package scheduler runs background tasks for the VirtBBS server. Currently
// just the FidoNet poll scheduler — see Start.
package scheduler

import (
	"log"
	"time"

	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
)

// Start launches one background goroutine per enabled FidoNet network that
// has a configured uplink, automatically polling — and immediately tossing
// afterward via fido.PollAndToss — on that network's effective interval
// (NetworkDef.EffectivePollInterval: 6 hours by default, overridable per
// network via poll_interval_mins, clamped to a 5-minute minimum).
//
// The set of networks is snapshotted once at startup; a network added at
// runtime (e.g. via the config.update API) won't get its own scheduler
// goroutine until the server restarts. Each goroutine re-reads its
// network's live config on every tick, so enabling/disabling a network,
// changing its uplink, or changing its poll interval takes effect on the
// next tick without a restart.
//
// Returns a stop function that halts all scheduler goroutines.
//
// VirtNet: networks this BBS hosts (NetworkDef.IsHub(), no uplink) get a
// daily fido.RunDayRollover ticker (full nodelist + diff generation,
// distribution, change log, diagrams) instead of the fetch-based nodelist
// scheduler below, which only makes sense for networks pulling someone
// else's nodelist.
func Start(store *messages.Store, confStore *conferences.Store, fileStore *files.Store) (stop func()) {
	cfg := config.Get()
	stopCh := make(chan struct{})
	var stopped bool

	for _, nd := range cfg.Fido.AllNetworks() {
		if !nd.Enabled {
			continue
		}
		name := nd.Name
		if nd.Uplink != "" {
			go runNetwork(name, store, confStore, stopCh)
			go runNodelistFetch(name, store, stopCh)
		} else {
			go runDayRollover(name, store, confStore, fileStore, stopCh)
		}
	}

	return func() {
		if !stopped {
			stopped = true
			close(stopCh)
		}
	}
}

// runDayRollover regenerates a hub network's VirtNet nodelist/diff/change-
// log/diagrams once every 24h (and once immediately at startup), and
// drains any pending inbound nodelist echoes (fido.ProcessPendingNodelistEchoes)
// once per minute — fast enough that a freshly-tossed echo is applied
// locally well within the same session a sysop might check it in.
func runDayRollover(networkName string, store *messages.Store, confStore *conferences.Store, fileStore *files.Store, stop <-chan struct{}) {
	nd := config.Get().Fido.NetworkByName(networkName)
	if nd == nil {
		return
	}
	log.Printf("virtnet scheduler: %s — day-rollover nodelist generation, daily", networkName)

	runRollover := func() {
		cfg := config.Get()
		nd := cfg.Fido.NetworkByName(networkName)
		if nd == nil || !nd.Enabled || !nd.IsHub() {
			return
		}
		warnings := fido.RunDayRollover(nd, store.DB(), confStore, store, fileStore, cfg.BBS.Name, cfg.Sysop.Name)
		for _, w := range warnings {
			log.Printf("virtnet scheduler: %s rollover warning: %s", networkName, w)
		}
	}
	runRollover() // once at startup, so a freshly-configured hub has a nodelist immediately

	dayTicker := time.NewTicker(24 * time.Hour)
	defer dayTicker.Stop()
	echoTicker := time.NewTicker(1 * time.Minute)
	defer echoTicker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-dayTicker.C:
			runRollover()
		case <-echoTicker.C:
			nd := config.Get().Fido.NetworkByName(networkName)
			if nd == nil || !nd.Enabled || !nd.IsHub() {
				continue
			}
			for _, w := range fido.ProcessPendingNodelistEchoes(store.DB(), fileStore) {
				log.Printf("virtnet scheduler: %s nodelist echo: %s", networkName, w)
			}
		}
	}
}

// runNetwork polls and tosses one network on its own ticker until stop is
// closed, re-reading live config every tick.
func runNetwork(networkName string, store *messages.Store, confStore *conferences.Store, stop <-chan struct{}) {
	nd := config.Get().Fido.NetworkByName(networkName)
	if nd == nil {
		return
	}
	interval := nd.EffectivePollInterval()
	log.Printf("fido scheduler: %s — polling every %s", networkName, interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cfg := config.Get()
			nd := cfg.Fido.NetworkByName(networkName)
			if nd == nil || !nd.Enabled || nd.Uplink == "" {
				// Disabled or removed at runtime — skip this tick, keep
				// waiting in case it's re-enabled later.
				continue
			}

			if newInterval := nd.EffectivePollInterval(); newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("fido scheduler: %s — interval changed to %s", networkName, interval)
			}

			result := fido.PollAndToss(nd, store, confStore, config.Get().Sysop.Name)
			if result.Poll.Error != nil {
				log.Printf("fido scheduler: %s poll error: %v", networkName, result.Poll.Error)
				continue
			}
			log.Printf("fido scheduler: %s poll complete (sent %d, received %d)",
				networkName, len(result.Poll.Sent), len(result.Poll.Received))

			if result.Toss != nil {
				log.Printf("fido scheduler: %s toss complete (%d imported, %d skipped, %d held)",
					networkName, result.Toss.Imported, result.Toss.Skipped, result.Toss.Orphaned)
				for _, e := range result.Toss.Errors {
					log.Printf("fido scheduler: %s toss error: %s", networkName, e)
				}
			}
		}
	}
}

// runNodelistFetch downloads and imports a fresh nodelist for one network
// on its own ticker until stop is closed, re-reading live config every
// tick (see fido.FetchAndImport). Independent of the poll ticker above —
// a network without an uplink configured can still want a current
// nodelist for address lookups.
func runNodelistFetch(networkName string, store *messages.Store, stop <-chan struct{}) {
	nd := config.Get().Fido.NetworkByName(networkName)
	if nd == nil {
		return
	}
	interval := nd.EffectiveNodelistInterval()
	log.Printf("nodelist scheduler: %s — fetching every %s from %s",
		networkName, interval, nd.EffectiveNodelistURL())

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			cfg := config.Get()
			nd := cfg.Fido.NetworkByName(networkName)
			if nd == nil || !nd.Enabled {
				continue
			}

			if newInterval := nd.EffectiveNodelistInterval(); newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("nodelist scheduler: %s — interval changed to %s", networkName, interval)
			}

			result, err := fido.FetchAndImport(nd, store.DB())
			if err != nil {
				log.Printf("nodelist scheduler: %s fetch error: %v", networkName, err)
				continue
			}
			log.Printf("nodelist scheduler: %s import complete (%d inserted, %d updated, %d skipped)",
				networkName, result.Inserted, result.Updated, result.Skipped)
			for _, e := range result.Errors {
				log.Printf("nodelist scheduler: %s import error: %s", networkName, e)
			}
		}
	}
}
