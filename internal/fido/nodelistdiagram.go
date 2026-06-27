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
//   v0.13.0 2026-06-27  VirtNet: Go rewrite of a Python FidoNet-nodelist-
//                        to-Graphviz-DOT converter (node2dot.py, see
//                        https://gist.github.com/ftoledo/8c17113d30e847a5e69f92bf0bbab82c),
//                        sourced from fido_members instead of a parsed
//                        nodelist file: full hierarchy, hubs-only topology,
//                        and one diagram per hub+its members, rendered to
//                        PNG via the external `dot` CLI (same as the source
//                        script — neither it nor Go has a built-in
//                        Graphviz renderer, so writing one from scratch
//                        isn't justified).
// ============================================================================

// Package fido — nodelistdiagram.go
package fido

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DiagramScope selects which subset of the network hierarchy buildDOT draws.
type DiagramScope int

const (
	// DiagramFull draws the complete hierarchy: zone -> every net's host -> every member.
	DiagramFull DiagramScope = iota
	// DiagramHubsOnly draws zone -> every net's host, omitting member nodes (topology skeleton).
	DiagramHubsOnly
	// DiagramPerHub draws one net's host plus only its direct members.
	DiagramPerHub
)

// buildDOT renders members (already filtered to a single net for
// DiagramPerHub) into Graphviz DOT text, mirroring node2dot.py's
// conventions: lightblue zone node, palegreen host nodes, lightpink member
// nodes, labels of "addr\nbbs_name\nsysop_name".
func buildDOT(zoneAddr Addr, hubBBSName, hubSysopName string, byNet map[int][]*Member, scope DiagramScope, onlyNet int) string {
	var b strings.Builder
	b.WriteString("digraph VirtNet {\r\n")
	b.WriteString("  node [shape=box, style=filled];\r\n")

	zoneID := "zone"
	fmt.Fprintf(&b, "  %s [label=\"%d\\n%s\\n%s\", fillcolor=lightblue];\r\n",
		zoneID, zoneAddr.Zone, dotEscape(hubBBSName), dotEscape(hubSysopName))

	nets := sortedNets(byNet)
	for _, net := range nets {
		if scope == DiagramPerHub && net != onlyNet {
			continue
		}
		members := byNet[net]
		host := findHost(members)

		hostID := fmt.Sprintf("host_%d", net)
		hostName, hostSysop := hubBBSName, hubSysopName
		if host != nil {
			hostName, hostSysop = host.BBSName, host.SysopName
		}
		fmt.Fprintf(&b, "  %s [label=\"%d:%d/%s\\n%s\\n%s\", fillcolor=palegreen];\r\n",
			hostID, zoneAddr.Zone, net, hostNodeNum(host), dotEscape(hostName), dotEscape(hostSysop))
		fmt.Fprintf(&b, "  %s -> %s;\r\n", zoneID, hostID)

		if scope == DiagramHubsOnly {
			continue
		}
		for _, m := range members {
			if host != nil && m.ID == host.ID {
				continue
			}
			memberID := fmt.Sprintf("m_%d", m.ID)
			fmt.Fprintf(&b, "  %s [label=\"%s\\n%s\\n%s\", fillcolor=lightpink];\r\n",
				memberID, dotEscape(m.Addr4D()), dotEscape(m.BBSName), dotEscape(m.SysopName))
			fmt.Fprintf(&b, "  %s -> %s;\r\n", hostID, memberID)
		}
	}

	b.WriteString("}\r\n")
	return b.String()
}

func hostNodeNum(host *Member) string {
	if host == nil {
		return "1"
	}
	return fmt.Sprintf("%d", host.NodeNum)
}

func dotEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`)
}

// RenderPNG writes dot to a temp .dot file and shells out to the `dot` CLI
// to render it to outPath as a PNG. If `dot` isn't found on PATH, returns
// a descriptive error rather than panicking — the caller should log it and
// skip that diagram, not fail the whole regeneration (see GenerateDiagrams).
func RenderPNG(dot, outPath string) error {
	dotBin, err := exec.LookPath("dot")
	if err != nil {
		return fmt.Errorf("graphviz 'dot' not found on PATH: %w", err)
	}
	tmp, err := os.CreateTemp("", "virtnet-*.dot")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(dot); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	cmd := exec.Command(dotBin, "-Tpng", tmp.Name(), "-o", outPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("dot render failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// GenerateDiagrams builds the full, hubs-only, and one-per-net DOT graphs
// for network from members, renders each to a PNG (skipping any that fail
// — e.g. `dot` not installed — with the returned warnings), and returns a
// map of {filename: pngBytes} ready for files.WriteMultiZipWithDiz, plus
// any non-fatal warnings encountered along the way.
func GenerateDiagrams(zoneAddr Addr, hubBBSName, hubSysopName string, members []*Member) (map[string][]byte, []string) {
	byNet := groupByNet(members)
	pngs := map[string][]byte{}
	var warnings []string

	render := func(name, dot string) {
		tmpOut, err := os.CreateTemp("", "virtnet-*.png")
		if err != nil {
			warnings = append(warnings, err.Error())
			return
		}
		defer os.Remove(tmpOut.Name())
		tmpOut.Close()
		if err := RenderPNG(dot, tmpOut.Name()); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
			return
		}
		data, err := os.ReadFile(tmpOut.Name())
		if err != nil {
			warnings = append(warnings, err.Error())
			return
		}
		pngs[name] = data
	}

	render("VirtNet_Full.png", buildDOT(zoneAddr, hubBBSName, hubSysopName, byNet, DiagramFull, 0))
	render("VirtNet_Hubs.png", buildDOT(zoneAddr, hubBBSName, hubSysopName, byNet, DiagramHubsOnly, 0))
	for _, net := range sortedNets(byNet) {
		name := fmt.Sprintf("Hub_%d-%d.png", zoneAddr.Zone, net)
		render(name, buildDOT(zoneAddr, hubBBSName, hubSysopName, byNet, DiagramPerHub, net))
	}

	return pngs, warnings
}
