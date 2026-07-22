// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"strings"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/application/utils"
)

type snapHintSuite struct{}

func TestSnapConfinementHintSuite(t *testing.T) {
	tc.Run(t, &snapHintSuite{})
}

func (s *snapHintSuite) TestSnapConfinementHint(c *tc.C) {
	const (
		snapEnv        = "/snap/juju/current"
		snapRealHome   = "/home/user"
		homeDir        = "/home/user"
		snapUserData   = "/home/user/snap/juju/current"
		snapUserCommon = "/home/user/snap/juju/common"
	)

	tests := []struct {
		desc           string
		path           string
		snapEnv        string
		snapUserData   string
		snapUserCommon string
		wantHint       bool
	}{
		{
			// (a) path under HOME, snap set - genuine not-found, no hint
			desc:     "path under HOME snap set",
			path:     homeDir + "/charms/foo.charm",
			snapEnv:  snapEnv,
			wantHint: false,
		},
		{
			// (b) path under /tmp, snap set - confinement hint
			desc:     "path under /tmp snap set",
			path:     "/tmp/foo.charm",
			snapEnv:  snapEnv,
			wantHint: true,
		},
		{
			// (c) path under /tmp, snap NOT set - no hint
			desc:     "path under /tmp snap not set",
			path:     "/tmp/foo.charm",
			snapEnv:  "",
			wantHint: false,
		},
		{
			// (d) charmhub-style name (no '/') - no hint
			desc:     "charmhub name no slash",
			path:     "mysql",
			snapEnv:  snapEnv,
			wantHint: false,
		},
		{
			// (e) path under /mnt, snap set - confinement hint
			desc:     "path under /mnt snap set",
			path:     "/mnt/usb/foo.charm",
			snapEnv:  snapEnv,
			wantHint: true,
		},
		{
			// (f) path under $SNAP_USER_DATA, snap set - no hint
			desc:         "path under SNAP_USER_DATA snap set",
			path:         snapUserData + "/foo.charm",
			snapEnv:      snapEnv,
			snapUserData: snapUserData,
			wantHint:     false,
		},
		{
			// (g) path under $SNAP_USER_COMMON, snap set - no hint
			desc:           "path under SNAP_USER_COMMON snap set",
			path:           snapUserCommon + "/foo.charm",
			snapEnv:        snapEnv,
			snapUserCommon: snapUserCommon,
			wantHint:       false,
		},
	}

	for _, tt := range tests {
		c.Logf("case: %s", tt.desc)
		hint := utils.SnapConfinementHint(tt.path, tt.snapEnv, snapRealHome, homeDir, tt.snapUserData, tt.snapUserCommon)
		if tt.wantHint {
			c.Check(hint, tc.Not(tc.Equals), "",
				tc.Commentf("expected hint for path %q", tt.path))
			c.Check(strings.Contains(hint, tt.path), tc.Equals, true,
				tc.Commentf("hint should contain the path %q", tt.path))
		} else {
			c.Check(hint, tc.Equals, "",
				tc.Commentf("expected no hint for path %q", tt.path))
		}
	}
}
