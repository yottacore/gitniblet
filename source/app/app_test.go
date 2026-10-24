// SPDX-License-Identifier: AGPL-3.0-only
// Copyright © 2026 Jim Collier [ID: 2უNაɘ«҂թȹɤξπ๙¿ձϖ]

package app

import (
	"testing"
	"time"
)

func TestVersionLine(t *testing.T) {
	oldBuild, oldTime, oldLocal := Build, BuildTime, time.Local
	t.Cleanup(func() { Build, BuildTime, time.Local = oldBuild, oldTime, oldLocal })

	cases := []struct {
		build, buildTime, zone string
		offset                 int
		want                   string
	}{
		{"", "", "UTC", 0, "ngdb " + Version},
		{"abc1234", "", "UTC", 0, "ngdb " + Version + " (abc1234)"},
		{"abc1234", "garbage", "UTC", 0, "ngdb " + Version + " (abc1234)"},
		{"abc1234", "2026-10-24T03:00:00Z", "UTC", 0, "ngdb " + Version + " (abc1234) (2026-10-24)"},
		// Same build seen from the US west coast is still the 23rd there.
		{"abc1234", "2026-10-24T03:00:00Z", "PDT", -7 * 3600, "ngdb " + Version + " (abc1234) (2026-10-23)"},
		{"", "2026-10-24T03:00:00Z", "UTC", 0, "ngdb " + Version + " (2026-10-24)"},
	}
	for _, c := range cases {
		Build, BuildTime = c.build, c.buildTime
		time.Local = time.FixedZone(c.zone, c.offset)
		if got := VersionLine(); got != c.want {
			t.Errorf("Build=%q BuildTime=%q zone=%s: got %q, want %q", c.build, c.buildTime, c.zone, got, c.want)
		}
	}
}
