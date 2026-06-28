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
//   v0.0.2  2026-06-24  Phase 10: Handler signatures updated to io.ReadWriteCloser
//   v0.0.5  2026-06-24  Phase 12/14: doors config, CallerLog path from config
//   v0.6.0  2026-06-26  Phase 0 (VirtAnd/VirtTerm): start internal/userapi listener
//                        on cfg.Network.UserAPIBind:UserAPIPort
//   v0.12.0 2026-06-27  Detect a vanished underlying volume (USB/external
//                        drive ejected while running) via a watchVolume
//                        goroutine and exit gracefully instead of spinning.
//   v0.13.0 2026-06-28  Start internal/web HTTP listener on WebBind:WebPort
// ============================================================================

// virtbbs is the VirtBBS server — Telnet + SSH BBS with web UI and VirtAnd user API.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"

	"github.com/virtbbs/virtbbs/internal/callers"
	"github.com/virtbbs/virtbbs/internal/conferences"
	"github.com/virtbbs/virtbbs/internal/config"
	"github.com/virtbbs/virtbbs/internal/db"
	"github.com/virtbbs/virtbbs/internal/fido"
	"github.com/virtbbs/virtbbs/internal/files"
	"github.com/virtbbs/virtbbs/internal/messages"
	"github.com/virtbbs/virtbbs/internal/node"
	"github.com/virtbbs/virtbbs/internal/scheduler"
	"github.com/virtbbs/virtbbs/internal/session"
	"github.com/virtbbs/virtbbs/internal/sshsrv"
	"github.com/virtbbs/virtbbs/internal/telnet"
	"github.com/virtbbs/virtbbs/internal/userapi"
	"github.com/virtbbs/virtbbs/internal/users"
	"github.com/virtbbs/virtbbs/internal/version"
	"github.com/virtbbs/virtbbs/internal/web"
)

func main() {
	cfgPath      := flag.String("config",        "VirtBBS.DAT", "path to VirtBBS.DAT")
	initSysop    := flag.Bool("init-sysop",      false,         "create/reset sysop account and exit")
	showVer      := flag.Bool("version",         false,         "print version and exit")
	importCfg    := flag.String("import-config", "",            "import PCBOARD.DAT from this path and exit")
	importUsers  := flag.String("import-users",  "",            "import PCBoard USERS binary from this path and exit")
	importMsgs   := flag.String("import-msgs",   "",            "import PCBoard MSGS binary from this path and exit")
	importMsgCon := flag.Int("import-msgs-conf", 0,             "target conference ID for --import-msgs (default 0)")
	fidoToss          := flag.Bool("fido-toss",            false, "toss all inbound .PKT files and exit")
	fidoScan          := flag.Bool("fido-scan",            false, "scan echo messages to outbound .PKT and exit")
	fidoFileScan      := flag.Bool("fido-filescan",        false, "scan file areas to outbound TIC and exit")
	fidoRebuildMaps   := flag.Bool("fido-rebuild-maps",    false, "rebuild VirtNet network map diagrams (VirtDiag.zip) and exit")
	fidoRebuildMapsNet := flag.String("fido-rebuild-maps-net", "", "network name for --fido-rebuild-maps (default: first hub network)")
	importNodelist    := flag.String("import-nodelist",    "",    "import a NODELIST.xxx file (or dir) and exit")
	importNodelistNet := flag.String("import-nodelist-net","FidoNet", "network name for --import-nodelist")
	flag.Parse()

	if *showVer {
		fmt.Printf("VirtBBS %s\n", version.Version)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// Ensure data directories exist
	for _, dir := range []string{"data", cfg.Paths.Files, cfg.Paths.Logs, cfg.Paths.WWW} {
		_ = os.MkdirAll(dir, 0755)
	}

	// Open shared SQLite database (WAL + single pool for all stores).
	sqlDB, err := db.Open(cfg.Paths.DB)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()

	userStore, err := users.Open(sqlDB)
	if err != nil {
		log.Fatalf("users store: %v", err)
	}
	defer userStore.Close()

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// ── One-shot import / fido commands ─────────────────────────────────────
	if *initSysop {
		if err := runInitSysop(cfg, userStore); err != nil {
			log.Fatal(err)
		}
		return
	}

	if *importCfg != "" {
		newCfg, err := config.ImportPCBoardDAT(*importCfg)
		if err != nil {
			log.Fatalf("import-config: %v", err)
		}
		// Preserve network/paths/sysop — only update BBS fields.
		cfg.BBS.Name = newCfg.BBS.Name
		cfg.BBS.MaxNodes = newCfg.BBS.MaxNodes
		if newCfg.Sysop.Name != "" {
			cfg.Sysop.Name = newCfg.Sysop.Name
		}
		if err := config.Save(cfg); err != nil {
			log.Fatalf("save config: %v", err)
		}
		fmt.Printf("Imported config from %s — BBS name: %q, max nodes: %d\n",
			*importCfg, cfg.BBS.Name, cfg.BBS.MaxNodes)
		return
	}

	if *importUsers != "" {
		imported, skipped, err := users.ImportUSERS(userStore, *importUsers)
		if err != nil {
			log.Fatalf("import-users: %v", err)
		}
		fmt.Printf("Imported %d users, skipped %d from %s\n", imported, skipped, *importUsers)
		return
	}

	if *importMsgs != "" {
		msgStore, err := messages.Open(sqlDB)
		if err != nil {
			log.Fatalf("messages store: %v", err)
		}
		defer msgStore.Close()
		imported, skipped, err := messages.ImportMSGS(msgStore, *importMsgCon, *importMsgs)
		if err != nil {
			log.Fatalf("import-msgs: %v", err)
		}
		fmt.Printf("Imported %d messages, skipped %d from %s into conference %d\n",
			imported, skipped, *importMsgs, *importMsgCon)
		return
	}

	if *fidoToss || *fidoScan || *fidoFileScan || *fidoRebuildMaps {
		msgStore, err := messages.Open(sqlDB)
		if err != nil {
			log.Fatalf("messages store: %v", err)
		}
		defer msgStore.Close()
		fidoCfg := &cfg.Fido

		confStore, _ := conferences.Open(sqlDB)
		if confStore != nil {
			defer confStore.Close()
		}
		fileStore, _ := files.Open(sqlDB, cfg.Paths.Files)
		var fileArea fido.FileArea
		if fileStore != nil {
			fileArea = fileStore
		}

		if *fidoToss {
			result := fido.TossAll(fidoCfg, msgStore, confStore, cfg.Sysop.Name, fileArea, cfg.Paths.Files)
			fmt.Printf("Toss complete: %d packets, %d imported, %d skipped, %d held, %d TIC\n",
				result.Packets, result.Imported, result.Skipped, result.Orphaned, result.TICProcessed)
			for _, e := range result.Errors {
				fmt.Fprintln(os.Stderr, "  ERROR:", e)
			}
		}

		if *fidoScan {
			result, err := fido.ScanAll(fidoCfg, msgStore, confStore, cfg.BBS.Name)
			if err != nil {
				log.Fatalf("fido-scan: %v", err)
			}
			fmt.Printf("Scan complete: %d messages exported in %d PKT(s)\n",
				result.Scanned, result.PKTFiles)
			for _, e := range result.Errors {
				fmt.Fprintln(os.Stderr, "  ERROR:", e)
			}
		}

		if *fidoFileScan {
			result, err := fido.FileScanAll(fidoCfg, sqlDB, cfg.Paths.Files)
			if err != nil {
				log.Fatalf("fido-filescan: %v", err)
			}
			fmt.Printf("File scan complete: %d file(s) in %d TIC ticket(s)\n",
				result.Files, result.TICFiles)
			for _, e := range result.Errors {
				fmt.Fprintln(os.Stderr, "  ERROR:", e)
			}
		}

		if *fidoRebuildMaps {
			netName := strings.TrimSpace(*fidoRebuildMapsNet)
			var nd *fido.NetworkDef
			if netName != "" {
				if n := fidoCfg.NetworkByName(netName); n != nil {
					nd = n
				} else {
					log.Fatalf("fido-rebuild-maps: network %q not found", netName)
				}
			} else {
				for _, n := range fidoCfg.AllNetworks() {
					if n.Enabled && n.IsHub() {
						ndCopy := n
						nd = &ndCopy
						break
					}
				}
				if nd == nil {
					log.Fatal("fido-rebuild-maps: no enabled hub network found — set --fido-rebuild-maps-net")
				}
			}
			count, warns := fido.RebuildNetworkDiagrams(nd, sqlDB, fileArea, cfg.BBS.Name, cfg.Sysop.Name)
			if count == 0 && len(warns) > 0 {
				log.Fatalf("fido-rebuild-maps: %s", strings.Join(warns, "; "))
			}
			fmt.Printf("Network maps rebuilt for %s: %d diagram(s) in VirtDiag.zip\n", nd.Name, count)
			for _, w := range warns {
				fmt.Fprintln(os.Stderr, "  WARNING:", w)
			}
		}
		return
	}

	if *importNodelist != "" {
		msgStore, err := messages.Open(sqlDB)
		if err != nil {
			log.Fatalf("messages store: %v", err)
		}
		defer msgStore.Close()
		fmt.Printf("Importing nodelist from %s (network=%s)…\n", *importNodelist, *importNodelistNet)
		result, err := fido.ImportFile(msgStore.DB(), *importNodelist, *importNodelistNet)
		if err != nil {
			log.Fatalf("import-nodelist: %v", err)
		}
		fmt.Printf("Done: %d inserted, %d skipped, %d errors\n",
			result.Inserted, result.Skipped, len(result.Errors))
		for _, e := range result.Errors {
			fmt.Fprintln(os.Stderr, "  ERROR:", e)
		}
		return
	}

	msgStore, err := messages.Open(sqlDB)
	if err != nil {
		log.Fatalf("messages store: %v", err)
	}
	defer msgStore.Close()

	nodeStore, err := node.Open(sqlDB)
	if err != nil {
		log.Fatalf("node store: %v", err)
	}
	if err := nodeStore.ClearAll(); err != nil {
		log.Fatalf("node store clear: %v", err)
	}

	callerLogPath := cfg.Paths.CallerLog
	if callerLogPath == "" {
		callerLogPath = cfg.Paths.Logs + "/CALLERS.LOG"
	}
	callersLog, err := callers.Open(callerLogPath)
	if err != nil {
		log.Fatalf("callers log: %v", err)
	}

	if err := fido.InitBinkpLog(filepath.Join(cfg.Paths.Logs, "binkp.log")); err != nil {
		log.Fatalf("binkp log: %v", err)
	}
	fido.InitBinkpStats(sqlDB)

	fileStore, err := files.Open(sqlDB, cfg.Paths.Files)
	if err != nil {
		log.Fatalf("files store: %v", err)
	}
	defer fileStore.Close()

	confStore, err := conferences.Open(sqlDB)
	if err != nil {
		log.Fatalf("conferences store: %v", err)
	}
	defer confStore.Close()

	deps := session.Deps{
		Users:       userStore,
		Messages:    msgStore,
		Nodes:       nodeStore,
		Callers:     callersLog,
		Files:       fileStore,
		Conferences: confStore,
	}

	telnetHandler := func(rw io.ReadWriteCloser, remoteAddr string) {
		session.Run(rw, remoteAddr, deps, true) // Telnet: server echoes
	}
	sshHandler := func(rw io.ReadWriteCloser, remoteAddr string) {
		session.Run(rw, remoteAddr, deps, false) // SSH: PTY echoes
	}

	// Start Telnet server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Network.TelnetPort)
		log.Printf("Telnet listening on %s", addr)
		srv := &telnet.Server{Addr: addr, Handler: telnetHandler}
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("Telnet error: %v", err)
		}
	}()

	// Start SSH server
	go func() {
		addr := fmt.Sprintf(":%d", cfg.Network.SSHPort)
		log.Printf("SSH listening on %s", addr)
		srv := &sshsrv.Server{
			Addr:        addr,
			HostKeyFile: "data/host_key.pem",
			ValidatePassword: func(user, pass string) bool {
				_, err := userStore.Authenticate(user, pass)
				return err == nil
			},
			Handler: sshHandler,
		}
		if err := srv.ListenAndServe(); err != nil {
			log.Printf("SSH error: %v", err)
		}
	}()

	files.StartDailyLocalFile(fileStore)

	// Start the automatic FidoNet poll/toss scheduler (one goroutine per
	// enabled network with a configured uplink — see internal/scheduler).
	if cfg.Fido.Enabled {
		scheduler.Start(msgStore, confStore, fileStore)

		// Start the BinkP server so other systems (our uplink, or our own
		// downlinks) can poll THIS BBS instead of only the reverse.
		if _, err := fido.ServeBinkP(&cfg.Fido, msgStore, confStore, cfg.Sysop.Name, fileStore, cfg.Paths.Files); err != nil {
			fido.LogBinkp(fmt.Sprintf("BinkP server error: %v", err))
		}

		fido.StartBinkpStatsBulletins(sqlDB, cfg.Session.DisplayDir, cfg.BBS.Name)

		fido.EnsureAllNetworkOwnNodes(sqlDB, cfg.Fido.AllNetworks(), cfg.BBS.Name, cfg.Sysop.Name, cfg.Network.TelnetPort)
	}

	log.Printf("VirtBBS %s starting", version.Version)

	// User API (VirtAnd) — token-authenticated JSON-over-TCP on a separate port.
	userAPIAddr := fmt.Sprintf("%s:%d", cfg.Network.UserAPIBind, cfg.Network.UserAPIPort)
	log.Printf("User API (VirtAnd) listening on %s", userAPIAddr)
	userAPIDeps := userapi.Deps{
		Users:       userStore,
		Messages:    msgStore,
		Conferences: confStore,
		Files:       fileStore,
	}
	userAPISrv := &userapi.Server{Addr: userAPIAddr, Deps: userAPIDeps}

	// Start the browser-based BBS web UI (templates/static from paths.www).
	webAddr := fmt.Sprintf("%s:%d", cfg.Network.WebBind, cfg.Network.WebPort)
	log.Printf("Web UI listening on %s (www: %s)", webAddr, cfg.Paths.WWW)
	webDeps := web.Deps{
		Users:       userStore,
		Messages:    msgStore,
		Conferences: confStore,
		Files:       fileStore,
		Nodes:       nodeStore,
		Callers:     callersLog,
	}
	webSrv := &web.Server{Addr: webAddr, Root: cfg.Paths.WWW, Deps: webDeps}
	go func() {
		if err := webSrv.ListenAndServe(); err != nil {
			log.Printf("Web UI error: %v", err)
		}
	}()

	go watchVolume(cfg.Paths.DB)

	log.Fatal(userAPISrv.ListenAndServe())
}

// watchVolume periodically confirms dbPath (and its containing directory)
// is still reachable, and exits the process with a clear message if it
// isn't — e.g. the external/USB drive VirtBBS is running from gets
// ejected out from under it. Without this, a vanished volume left the
// process running indefinitely, pegging the CPU as SQLite/the OS kept
// retrying syscalls against a now-dead mount point on every connection and
// scheduler tick, with no way to stop it short of `kill -9` (observed
// directly: ~200-250% CPU, unkillable via plain SIGTERM, since nothing in
// the program ever checked for or reacted to the underlying filesystem
// disappearing).
//
// A repeated stat() failure is the simplest reliable signal available
// here: SQLite itself can report all sorts of driver-specific errors for
// a dead mount depending on OS/filesystem, but a missing/inaccessible
// regular file is unambiguous and cheap to check on a slow ticker.
func watchVolume(dbPath string) {
	const (
		checkInterval  = 5 * time.Second
		failuresToExit = 3
	)
	failures := 0
	for {
		time.Sleep(checkInterval)
		if _, err := os.Stat(dbPath); err != nil {
			failures++
			log.Printf("volume check: cannot stat database %q (%v) — %d/%d", dbPath, err, failures, failuresToExit)
			if failures >= failuresToExit {
				log.Fatalf("volume check: database %q unreachable for %d consecutive checks — "+
					"the underlying drive was likely disconnected. Exiting rather than spinning.",
					dbPath, failuresToExit)
			}
			continue
		}
		failures = 0
	}
}

// runInitSysop interactively creates or resets the sysop account and
// writes the bcrypt password hash into VirtBBS.DAT.
func runInitSysop(cfg *config.Config, store *users.Store) error {
	sc := bufio.NewScanner(os.Stdin)

	prompt := func(msg string) string {
		fmt.Print(msg)
		sc.Scan()
		return strings.TrimSpace(sc.Text())
	}

	fmt.Println("=== VirtBBS First-Run Sysop Setup ===")
	name := prompt("Sysop name [Sysop]: ")
	if name == "" {
		name = "Sysop"
	}

	fmt.Print("Password: ")
	passBytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	fmt.Print("Confirm password: ")
	pass2Bytes, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if string(passBytes) != string(pass2Bytes) {
		return fmt.Errorf("passwords do not match")
	}
	if len(passBytes) == 0 {
		return fmt.Errorf("password cannot be empty")
	}
	password := string(passBytes)

	// Create or update the sysop user record in the DB
	existing, _ := store.GetByName(name)
	if existing != nil {
		if err := store.SetPassword(existing.ID, password); err != nil {
			return fmt.Errorf("update sysop password: %w", err)
		}
		existing.Sysop = true
		existing.SecurityLevel = 110
		if err := store.Update(existing); err != nil {
			return fmt.Errorf("update sysop flags: %w", err)
		}
		fmt.Printf("Sysop account '%s' updated.\n", name)
	} else {
		u := &users.User{
			Name:          name,
			SecurityLevel: 110,
			PageLength:    24,
			XferProtocol:  "Z",
			ANSI:          true,
			Sysop:         true,
		}
		if err := store.Create(u, password); err != nil {
			return fmt.Errorf("create sysop: %w", err)
		}
		fmt.Printf("Sysop account '%s' created.\n", name)
	}

	// Hash the password and store it in VirtBBS.DAT for the API
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	cfg.Sysop.Name = name
	cfg.Sysop.PasswordHash = string(hash)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("save VirtBBS.DAT: %w", err)
	}
	fmt.Println("VirtBBS.DAT updated with sysop credentials.")
	fmt.Println("Setup complete — you can now start VirtBBS normally.")
	return nil
}
