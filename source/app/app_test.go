// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package app

import (
	"testing"
	"time"
)

// ID: debg5
func TestVersionLine(t *testing.T) {
	oldCommit, oldTime, oldLocal := CommitTime, BuildTime, time.Local
	t.Cleanup(func() { CommitTime, BuildTime, time.Local = oldCommit, oldTime, oldLocal })

	// 2026-10-24 22:00 UTC, build decp8.
	const commit = "1792879200"
	cases := []struct {
		commitTime, buildTime, zone string
		offset                      int
		want                        string
	}{
		{"", "", "UTC", 0, "gitniblet v" + Version},
		{commit, "", "UTC", 0, "gitniblet v" + Version + " build decp8"},
		{commit, "garbage", "UTC", 0, "gitniblet v" + Version + " build decp8"},
		{commit, "2026-10-24T03:00:00Z", "UTC", 0, "gitniblet v" + Version + " build decp8 (2026-10-24)"},
		// Same build seen from the US west coast is still the 23rd there.
		{commit, "2026-10-24T03:00:00Z", "PDT", -7 * 3600, "gitniblet v" + Version + " build decp8 (2026-10-23)"},
		{"", "2026-10-24T03:00:00Z", "UTC", 0, "gitniblet v" + Version + " (2026-10-24)"},
	}
	for _, c := range cases {
		CommitTime, BuildTime = c.commitTime, c.buildTime
		time.Local = time.FixedZone(c.zone, c.offset)
		if got := VersionLine(); got != c.want {
			t.Errorf("CommitTime=%q BuildTime=%q zone=%s: got %q, want %q", c.commitTime, c.buildTime, c.zone, got, c.want)
		}
	}
}

// ID: dedrs
func TestBuildNumber(t *testing.T) {
	old := CommitTime
	t.Cleanup(func() { CommitTime = old })

	for _, c := range []struct{ commitTime, want string }{
		{"", ""},
		{"garbage", ""},
		{"946684799", ""}, // 1999 has no number
		{"946684800", "0"},
		{"946684859", "0"}, // whole minutes, never rounded up
		{"946684860", "1"},
		{"946686720", "10"}, // minute 32
		{"1792879200", "decp8"},
		{"1792879259", "decp8"},
		{"1792879260", "decp9"},
	} {
		CommitTime = c.commitTime
		if got := BuildNumber(); got != c.want {
			t.Errorf("CommitTime=%q: got %q, want %q", c.commitTime, got, c.want)
		}
	}
}
