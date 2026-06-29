package fido

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// NodelistDaySuffix returns the two-digit Fido Z-suffix: day-of-year mod 100
// (e.g. 26 Jun → day 177 → "77").
func NodelistDaySuffix(t time.Time) string {
	return fmt.Sprintf("%02d", t.YearDay()%100)
}

// NodelistFullFilename returns the Fido full-nodelist name for t (NODELIST.Z##).
func NodelistFullFilename(t time.Time) string {
	return "NODELIST.Z" + NodelistDaySuffix(t)
}

// NodelistDiffFilename returns the Fido diff name for t (NODEDIFF.Z##).
func NodelistDiffFilename(t time.Time) string {
	return "NODEDIFF.Z" + NodelistDaySuffix(t)
}

// IsWeeklyNodelistDay reports when a full NODELIST.Z## file is published.
// FidoNet publishes the complete list weekly (Friday); other days use diffs only.
func IsWeeklyNodelistDay(t time.Time) bool {
	return t.Weekday() == time.Friday
}

// IsFullNodelistFilename reports whether name is a full nodelist (not a diff).
func IsFullNodelistFilename(name string) bool {
	upper := strings.ToUpper(filepathBase(name))
	switch {
	case strings.HasPrefix(upper, "NODELIST.Z"):
		return true
	case strings.HasPrefix(upper, "VIRTNODE.Z"):
		return true
	case strings.HasPrefix(upper, "NODELIST."):
		// Legacy NODELIST.### (three-digit day-of-year, not Z-form).
		return !strings.HasPrefix(upper, "NODELIST.Z")
	default:
		return false
	}
}

// IsNodelistDiffFilename reports whether name is a nodelist diff file.
func IsNodelistDiffFilename(name string) bool {
	upper := strings.ToUpper(filepathBase(name))
	switch {
	case strings.HasPrefix(upper, "NODEDIFF.Z"):
		return true
	case strings.HasPrefix(upper, "VIRTNODE.D"):
		return true
	case strings.HasPrefix(upper, "NODEDIFF."):
		return !strings.HasPrefix(upper, "NODEDIFF.Z")
	default:
		return false
	}
}

func filepathBase(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		return name[i+1:]
	}
	return name
}

// nodelistFilenameFromSubject maps echo subjects to on-disk Fido filenames.
// Accepts "VirtNet Nodelist Z77", "VirtNet Nodelist Diff Z79", and legacy
// VirtNode.Z045 / VirtNode.D045 forms.
func nodelistFilenameFromSubject(subject string) string {
	now := time.Now()
	fields := strings.Fields(subject)
	if len(fields) == 0 {
		return NodelistFullFilename(now)
	}
	last := strings.ToUpper(fields[len(fields)-1])
	isDiff := strings.Contains(strings.ToUpper(subject), " DIFF ")

	if len(last) >= 2 && (last[0] == 'Z' || last[0] == 'D') {
		suffix := last[1:]
		if _, err := strconv.Atoi(suffix); err == nil {
			if len(suffix) == 3 {
				if n, _ := strconv.Atoi(suffix); n > 0 {
					suffix = fmt.Sprintf("%02d", n%100)
				}
			}
			if isDiff || last[0] == 'D' {
				return "NODEDIFF.Z" + suffix
			}
			return "NODELIST.Z" + suffix
		}
	}
	if isDiff {
		return NodelistDiffFilename(now)
	}
	return NodelistFullFilename(now)
}

// nodelistBodyHasChanges reports whether a generated diff has lines beyond comments.
func nodelistBodyHasChanges(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		return true
	}
	return false
}

// ParseNodelistZSuffix extracts the two-digit Z suffix from a nodelist filename.
// Returns (-1, false) when the name has no recognised Z suffix.
func ParseNodelistZSuffix(filename string) (int, bool) {
	upper := strings.ToUpper(filepathBase(filename))
	for _, prefix := range []string{"NODELIST.Z", "NODEDIFF.Z", "VIRTNODE.Z", "VIRTNODE.D"} {
		if strings.HasPrefix(upper, prefix) {
			n, err := strconv.Atoi(upper[len(prefix):])
			if err != nil {
				return -1, false
			}
			return n, true
		}
	}
	return -1, false
}

// latestFullNodelistPath returns the newest full nodelist file in dir.
func latestFullNodelistPath(dir string) string {
	if p := latestNodelistFile(dir, "NODELIST.Z"); p != "" {
		return p
	}
	return latestNodelistFile(dir, "VirtNode.Z")
}

// latestNodelistDiffPath returns the newest diff file in dir.
func latestNodelistDiffPath(dir string) string {
	if p := latestNodelistFile(dir, "NODEDIFF.Z"); p != "" {
		return p
	}
	return latestNodelistFile(dir, "VirtNode.D")
}
