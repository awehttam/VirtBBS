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
//   v1.6.0  2026-06-28  Member networks: drain nodelist echo queue on
//                        startup, after poll+toss, and every 1 minute
//                        (ProcessPendingNodelistEchoesForNetwork).
// ============================================================================

// Package scheduler runs background tasks for the VirtBBS server. Currently
// just the FidoNet poll scheduler — see Start.
package scheduler

import (
	"fmt"
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
		go runNodelistMonitor(name, store, confStore, fileStore, stopCh)
		if nd.Uplink != "" {
			go runNetwork(name, store, confStore, fileStore, stopCh)
			if nd.NodelistFetchEnabled() {
				go runNodelistFetch(name, store, fileStore, stopCh)
			} else {
				log.Printf("nodelist scheduler: %s — automatic fetch disabled (no nodelist_url configured)", name)
			}
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
// log/diagrams once every 24h, and once immediately at startup (files only —
// no echo conference posts until the first daily rollover). Also drains any
// pending inbound nodelist echoes for this network once per minute.
func runDayRollover(networkName string, store *messages.Store, confStore *conferences.Store, fileStore *files.Store, stop <-chan struct{}) {
	nd := config.Get().Fido.NetworkByName(networkName)
	if nd == nil {
		return
	}
	log.Printf("virtnet scheduler: %s — day-rollover nodelist generation, daily", networkName)

	runRollover := func(publish bool) {
		cfg := config.Get()
		nd := cfg.Fido.NetworkByName(networkName)
		if nd == nil || !nd.Enabled || !nd.IsHub() {
			return
		}
		warnings := fido.RunDayRollover(nd, store.DB(), confStore, store, fileStore, cfg.BBS.Name, cfg.Sysop.Name, publish)
		for _, w := range warnings {
			log.Printf("virtnet scheduler: %s rollover warning: %s", networkName, w)
		}
	}
	runRollover(false) // startup: regenerate files locally, defer echo posts to daily rollover

	dayTicker := time.NewTicker(24 * time.Hour)
	defer dayTicker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-dayTicker.C:
			runRollover(true)
		}
	}
}

// runNodelistMonitor scans each network's Nodelist Files area and Nodelists
// conference every minute, applying nodelists/diffs newer than the current import.
func runNodelistMonitor(networkName string, store *messages.Store, confStore *conferences.Store, fileStore *files.Store, stop <-chan struct{}) {
	log.Printf("nodelist monitor: %s — checking file area and conference every 1m", networkName)
	drainPendingNodelistEchoes(networkName, store, confStore, fileStore)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			nd := config.Get().Fido.NetworkByName(networkName)
			if nd == nil || !nd.Enabled {
				continue
			}
			drainPendingNodelistEchoes(networkName, store, confStore, fileStore)
		}
	}
}

func drainPendingNodelistEchoes(networkName string, store *messages.Store, confStore *conferences.Store, fileStore *files.Store) {
	if fileStore == nil {
		return
	}
	cfg := config.Get()
	nd := cfg.Fido.NetworkByName(networkName)
	var ndPtr *fido.NetworkDef
	bbsName, sysopName := "", ""
	telnetPort := 0
	if nd != nil {
		ndCopy := *nd
		ndPtr = &ndCopy
		bbsName = cfg.BBS.Name
		sysopName = cfg.Sysop.Name
		telnetPort = cfg.Network.TelnetPort
	}
	for _, w := range fido.MonitorNetworkNodelists(store.DB(), confStore, store, fileStore, ndPtr, bbsName, sysopName, telnetPort) {
		log.Printf("nodelist monitor [%s]: %s", networkName, w)
	}
}

// runNetwork polls and tosses one network on its own ticker until stop is
// closed, re-reading live config every tick. After each poll+toss, TossDir
// applies any queued VirtNet nodelist echoes; a 1-minute backup ticker also
// drains the queue for this network (covers manual toss without poll).
func runNetwork(networkName string, store *messages.Store, confStore *conferences.Store, fileStore *files.Store, stop <-chan struct{}) {
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

			var fileArea fido.FileArea
			if fileStore != nil {
				fileArea = fileStore
			}
			result := fido.PollAndToss(nd, store, confStore, config.Get().Sysop.Name, fileArea, config.Get().Paths.Files)
			if result.Poll.Error != nil {
				fido.LogBinkp(fmt.Sprintf("fido scheduler: %s poll error: %v", networkName, result.Poll.Error))
				continue
			}
			fido.LogBinkp(fmt.Sprintf("fido scheduler: %s poll complete (sent %d, received %d)",
				networkName, len(result.Poll.Sent), len(result.Poll.Received)))

			if result.Toss != nil {
				fido.LogBinkp(fmt.Sprintf("fido scheduler: %s toss complete (%s)",
					networkName, result.Toss.TossSummary()))
				for _, e := range result.Toss.Errors {
					fido.LogBinkp(fmt.Sprintf("fido scheduler: %s toss error: %s", networkName, e))
				}
			}
			drainPendingNodelistEchoes(networkName, store, confStore, fileStore)
		}
	}
}

// runNodelistFetch downloads and imports a fresh nodelist for one network
// on its own ticker until stop is closed, re-reading live config every
// tick (see fido.FetchAndImport). After a successful import, network
// diagrams are regenerated into <Network>_diags.zip when a file store is
// available.
func runNodelistFetch(networkName string, store *messages.Store, fileStore *files.Store, stop <-chan struct{}) {
	nd := config.Get().Fido.NetworkByName(networkName)
	if nd == nil {
		return
	}
	if !nd.NodelistFetchEnabled() {
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
			if !nd.NodelistFetchEnabled() {
				continue
			}

			if newInterval := nd.EffectiveNodelistInterval(); newInterval != interval {
				interval = newInterval
				ticker.Reset(interval)
				log.Printf("nodelist scheduler: %s — interval changed to %s", networkName, interval)
			}

			result, err := fido.FetchAndImport(nd, store.DB(), fileStore)
			if err != nil {
				log.Printf("nodelist scheduler: %s fetch error: %v", networkName, err)
				continue
			}
			log.Printf("nodelist scheduler: %s import complete (%d inserted, %d updated, %d skipped)",
				networkName, result.Inserted, result.Updated, result.Skipped)
			for _, e := range result.Errors {
				log.Printf("nodelist scheduler: %s import error: %s", networkName, e)
			}
			if fileStore != nil && result.Inserted+result.Updated > 0 {
				cfg := config.Get()
				count, warns := fido.RebuildNetworkDiagrams(nd, store.DB(), fileStore, cfg.BBS.Name, cfg.Sysop.Name)
				if count > 0 {
					log.Printf("nodelist scheduler: %s — rebuilt %d network diagram(s)", networkName, count)
				}
				for _, w := range warns {
					log.Printf("nodelist scheduler: %s diagram: %s", networkName, w)
				}
			}
		}
	}
}
