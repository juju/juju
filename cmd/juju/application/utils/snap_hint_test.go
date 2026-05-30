// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"strings"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/utils"
)

type snapHintSuite struct{}

var _ = gc.Suite(&snapHintSuite{})

func (s *snapHintSuite) TestSnapConfinementHint(c *gc.C) {
	const (
		snapEnv      = "/snap/juju/current"
		snapRealHome = "/home/user"
		homeDir      = "/home/user"
	)

	tests := []struct {
		desc     string
		path     string
		snapEnv  string
		wantHint bool
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
	}

	for _, tt := range tests {
		c.Logf("case: %s", tt.desc)
		hint := utils.SnapConfinementHint(tt.path, tt.snapEnv, snapRealHome, homeDir)
		if tt.wantHint {
			c.Check(hint, gc.Not(gc.Equals), "",
				gc.Commentf("expected hint for path %q", tt.path))
			c.Check(strings.Contains(hint, tt.path), gc.Equals, true,
				gc.Commentf("hint should contain the path %q", tt.path))
		} else {
			c.Check(hint, gc.Equals, "",
				gc.Commentf("expected no hint for path %q", tt.path))
		}
	}
}
