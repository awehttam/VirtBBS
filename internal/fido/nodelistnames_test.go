package fido

import (
	"testing"
	"time"
)

func TestNodelistDaySuffix(t *testing.T) {
	// 26 Jun 2026 = day 177 → 177 % 100 = 77
	d := time.Date(2026, 6, 26, 12, 0, 0, 0, time.UTC)
	if got := NodelistDaySuffix(d); got != "77" {
		t.Fatalf("NodelistDaySuffix(2026-06-26) = %q, want 77", got)
	}
	// 28 Jun 2026 = day 179 → 79
	d2 := time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)
	if got := NodelistFullFilename(d2); got != "NODELIST.Z79" {
		t.Fatalf("NodelistFullFilename = %q, want NODELIST.Z79", got)
	}
}

func TestNodelistFilenameFromSubject(t *testing.T) {
	cases := []struct {
		subject string
		want    string
	}{
		{"VirtNet Nodelist Z77", "NODELIST.Z77"},
		{"VirtNet Nodelist Diff Z79", "NODEDIFF.Z79"},
		{"VirtNet Nodelist Diff D045", "NODEDIFF.Z45"},
		{"VirtNet Nodelist Z045", "NODELIST.Z45"},
	}
	for _, tc := range cases {
		if got := nodelistFilenameFromSubject(tc.subject); got != tc.want {
			t.Fatalf("subject %q → %q, want %q", tc.subject, got, tc.want)
		}
	}
}

func TestIsFullNodelistFilename(t *testing.T) {
	if !IsFullNodelistFilename("NODELIST.Z77") {
		t.Fatal("NODELIST.Z77 should be full")
	}
	if IsFullNodelistFilename("NODEDIFF.Z77") {
		t.Fatal("NODEDIFF.Z77 should not be full")
	}
	if !IsFullNodelistFilename("VirtNode.Z045") {
		t.Fatal("legacy VirtNode.Z045 should be full")
	}
}

func TestIsWeeklyNodelistDay(t *testing.T) {
	fri := time.Date(2026, 6, 26, 0, 0, 0, 0, time.UTC) // Friday
	sat := time.Date(2026, 6, 27, 0, 0, 0, 0, time.UTC)
	if !IsWeeklyNodelistDay(fri) {
		t.Fatal("Friday should be weekly nodelist day")
	}
	if IsWeeklyNodelistDay(sat) {
		t.Fatal("Saturday should not be weekly nodelist day")
	}
}
