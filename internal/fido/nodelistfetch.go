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
//   v0.5.0  2026-06-25  Initial implementation — automatic nodelist fetch,
//                        defaulting to scanning https://www.darkrealms.ca/
//                        for the current day's "Fidonet Daily Nodelist
//                        (Z1/ZIP) day NNN" download link
// ============================================================================

package fido

// Package fido — nodelistfetch.go
//
// Downloads a fresh FidoNet nodelist over HTTP(S) and imports it via the
// existing ImportFile(). NodelistURL may be:
//
//   - A direct file URL (recognised by extension: .zip/.lzh/.arc/.arj/.gz,
//     or a classic NODELIST.### numeric extension) — downloaded as-is.
//   - A "discovery page" (the default — see DefaultNodelistDiscoveryURL):
//     the page's HTML is scanned for a link whose text matches
//     "Fidonet Daily Nodelist (Z1/ZIP) day NNN", and that link's href is
//     resolved and downloaded. This is necessary because the real
//     download URL changes daily and isn't derivable from a fixed
//     pattern — it has to be read off the page fresh each time.
//
// The downloaded content is sniffed for a ZIP magic header rather than
// trusting the URL's file extension, since some sites (darkrealms.ca
// included) serve ZIP archives under misleading non-.zip extensions.

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var nodelistHTTPClient = &http.Client{Timeout: 60 * time.Second}

// rowSplitRe splits the discovery page's HTML into per-row chunks. The
// real page (darkrealms.ca) lays each file out as a row of <td> cells —
// filename link, description, size, date, hits — under an opening <tr>
// with no matching </tr>, so splitting on <tr...> boundaries is the
// reliable way to group a row's cells together.
var rowSplitRe = regexp.MustCompile(`(?i)<tr[^>]*>`)

// hrefRe finds the first href attribute within a row chunk.
var hrefRe = regexp.MustCompile(`(?is)href\s*=\s*"([^"]+)"`)

// dailyNodelistRe matches the description text VirtBBS looks for on the
// discovery page, e.g. "Fidonet Daily Nodelist (Z1/ZIP) day 177" — this
// appears in a separate <td> from the download link within the same row.
var dailyNodelistRe = regexp.MustCompile(`(?i)Fidonet\s+Daily\s+Nodelist\s*\(Z1/ZIP\)\s*day\s+(\d+)`)

// discoverNodelistURL fetches pageURL and returns the resolved href found
// in the first table row whose text matches dailyNodelistRe.
func discoverNodelistURL(pageURL string) (string, error) {
	resp, err := nodelistHTTPGet(pageURL)
	if err != nil {
		return "", fmt.Errorf("fetch discovery page %s: %w", pageURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read discovery page %s: %w", pageURL, err)
	}

	base, err := url.Parse(pageURL)
	if err != nil {
		return "", fmt.Errorf("parse discovery page URL %q: %w", pageURL, err)
	}

	for _, row := range rowSplitRe.Split(string(body), -1) {
		if !dailyNodelistRe.MatchString(row) {
			continue
		}
		m := hrefRe.FindStringSubmatch(row)
		if m == nil {
			continue
		}
		ref, err := url.Parse(m[1])
		if err != nil {
			continue
		}
		return base.ResolveReference(ref).String(), nil
	}
	return "", fmt.Errorf(`no "Fidonet Daily Nodelist (Z1/ZIP) day NNN" link found on %s`, pageURL)
}

// nodelistHTTPGet issues a GET with a sane timeout and User-Agent, and
// treats any non-200 response as an error.
func nodelistHTTPGet(rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "VirtBBS-nodelist-fetch/1.0")

	resp, err := nodelistHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	return resp, nil
}

// looksLikeDirectFile reports whether rawURL's path looks like a directly
// downloadable nodelist file rather than a page that needs to be scanned
// for a link: a recognised archive extension, a classic NODELIST.###
// 3-digit numeric extension, or the letter+2-digit form some download
// scripts use (e.g. darkrealms.ca's "Z1DAILY.Z77" — 'Z' + day-of-year mod
// 100, despite the response actually being a ZIP archive).
func looksLikeDirectFile(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	ext := strings.ToLower(filepath.Ext(u.Path))
	switch ext {
	case ".zip", ".lzh", ".arc", ".arj", ".gz":
		return true
	}
	// Last two characters being digits covers ".170" (NODELIST.170) and
	// ".z77"/".d77" (single-letter-prefixed day-of-year suffixes) alike.
	if len(ext) >= 3 {
		last2 := ext[len(ext)-2:]
		if last2[0] >= '0' && last2[0] <= '9' && last2[1] >= '0' && last2[1] <= '9' {
			return true
		}
	}
	return false
}

// FetchNodelist downloads the current nodelist for network nd — resolving
// nd.EffectiveNodelistURL() via the discovery-page scan above if it isn't
// already a direct file URL — and writes it into nd.NodelistDir. Returns
// the path to the resulting file (ready to pass to ImportFile).
func FetchNodelist(nd *NetworkDef) (string, error) {
	target := nd.EffectiveNodelistURL()

	if !looksLikeDirectFile(target) {
		discovered, err := discoverNodelistURL(target)
		if err != nil {
			return "", err
		}
		target = discovered
	}

	resp, err := nodelistHTTPGet(target)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", target, err)
	}

	if err := os.MkdirAll(nd.NodelistDir, 0755); err != nil {
		return "", err
	}

	// Sniff for a ZIP magic header rather than trusting the URL's
	// extension — e.g. darkrealms.ca's daily file is named like
	// "Z1DAILY.Z77" despite being a ZIP archive.
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		return extractNodelistZip(data, nd.NodelistDir)
	}

	name := filepath.Base(target)
	if name == "" || name == "." || name == "/" {
		name = "NODELIST.TMP"
	}
	destPath := filepath.Join(nd.NodelistDir, name)
	if err := os.WriteFile(destPath, data, 0644); err != nil {
		return "", err
	}
	return destPath, nil
}

// extractNodelistZip extracts the first file from a ZIP archive's bytes
// into destDir, returning its path.
func extractNodelistZip(data []byte, destDir string) (string, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("not a valid zip archive: %w", err)
	}
	if len(r.File) == 0 {
		return "", fmt.Errorf("zip archive is empty")
	}

	zf := r.File[0]
	rc, err := zf.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	destPath := filepath.Join(destDir, filepath.Base(zf.Name))
	if err := os.WriteFile(destPath, content, 0644); err != nil {
		return "", err
	}
	return destPath, nil
}

// FetchAndImport downloads the current nodelist for network nd and imports
// it via the existing ImportFile, using nd.Name as the logical network name.
func FetchAndImport(nd *NetworkDef, db *sql.DB) (*ImportResult, error) {
	path, err := FetchNodelist(nd)
	if err != nil {
		return nil, err
	}
	return ImportFile(db, path, nd.Name)
}
