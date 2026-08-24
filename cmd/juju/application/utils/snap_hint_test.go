// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/errors"
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
		homeDir        = "/home/user"
		snapUserData   = "/home/user/snap/juju/current"
		snapUserCommon = "/home/user/snap/juju/common"
	)

	// Each case spells out the whole environment rather than relying on shared
	// defaults, so that cases which deliberately leave a root unset (e.g. HOME
	// unset, or every root unset) are readable at a glance.
	tests := []struct {
		desc     string
		path     string
		env      utils.SnapEnv
		wantHint bool
	}{
		{
			// (a) path under HOME, snap set - genuine not-found, no hint
			desc:     "path under HOME snap set",
			path:     homeDir + "/charms/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: false,
		},
		{
			// (b) path under /tmp, snap set - confinement hint
			desc:     "path under /tmp snap set",
			path:     "/tmp/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: true,
		},
		{
			// (c) path under /tmp, snap NOT set - no hint
			desc:     "path under /tmp snap not set",
			path:     "/tmp/foo.charm",
			env:      utils.SnapEnv{RealHome: homeDir, Home: homeDir},
			wantHint: false,
		},
		{
			// (d) charmhub-style name (no '/') - no hint
			desc:     "charmhub name no slash",
			path:     "mysql",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: false,
		},
		{
			// (e) path under /mnt, snap set - confinement hint
			desc:     "path under /mnt snap set",
			path:     "/mnt/usb/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: true,
		},
		{
			// (f) tilde path, snap set - the shell would have expanded this to
			// the real home, so no hint
			desc:     "tilde path snap set",
			path:     "~/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: false,
		},
		{
			// (g) bare tilde, snap set - no hint (and no '/' either)
			desc:     "bare tilde snap set",
			path:     "~",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: false,
		},
		{
			// (h) path under $SNAP_USER_DATA, snap set - no hint
			desc:     "path under SNAP_USER_DATA snap set",
			path:     snapUserData + "/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir, UserData: snapUserData},
			wantHint: false,
		},
		{
			// (i) path under $SNAP_USER_COMMON, snap set - no hint
			desc:     "path under SNAP_USER_COMMON snap set",
			path:     snapUserCommon + "/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir, UserCommon: snapUserCommon},
			wantHint: false,
		},
		{
			// (j) $SNAP_REAL_HOME allowlists on its own, with HOME unset
			desc:     "path under SNAP_REAL_HOME with HOME unset",
			path:     homeDir + "/charms/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir},
			wantHint: false,
		},
		{
			// (k) path equal to a root, rather than under it. A root is only
			// reachable via a separator, so this is treated as unreachable.
			desc:     "path equal to HOME",
			path:     homeDir,
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: true,
		},
		{
			// (l) every root unset - nothing is reachable, so the hint fires
			desc:     "all roots unset",
			path:     "/tmp/foo.charm",
			env:      utils.SnapEnv{Snap: snapEnv},
			wantHint: true,
		},
		{
			// (m) trailing separator - cleaned before the root comparison, so
			// it behaves the same as (b)
			desc:     "unreachable path with trailing separator",
			path:     "/tmp/foo.charm/",
			env:      utils.SnapEnv{Snap: snapEnv, RealHome: homeDir, Home: homeDir},
			wantHint: true,
		},
	}

	for _, tt := range tests {
		c.Logf("case: %s", tt.desc)
		hint := utils.SnapConfinementHint(tt.path, tt.env)
		if tt.wantHint {
			c.Check(hint, tc.Not(tc.Equals), "",
				tc.Commentf("expected hint for path %q", tt.path))
			c.Check(hint, tc.Contains, tt.path,
				tc.Commentf("hint should contain the path %q", tt.path))
		} else {
			c.Check(hint, tc.Equals, "",
				tc.Commentf("expected no hint for path %q", tt.path))
		}
	}
}

// TestSnapConfinementHintRelativePaths checks that a relative path is resolved
// against the working directory before being compared with the snap's
// reachable roots, so that "./foo.charm" is judged by where the user actually
// is rather than by the literal string.
func (s *snapHintSuite) TestSnapConfinementHintRelativePaths(c *tc.C) {
	home := c.MkDir()
	sub := filepath.Join(home, "charms")
	c.Assert(os.Mkdir(sub, 0700), tc.ErrorIsNil)

	// From a directory under HOME, "./foo.charm" resolves to a reachable path.
	c.Chdir(sub)
	env := utils.SnapEnv{Snap: "/snap/juju/current", Home: home}
	c.Check(utils.SnapConfinementHint("./foo.charm", env), tc.Equals, "")

	// "../foo.charm" from that same directory is still under HOME.
	c.Check(utils.SnapConfinementHint("../foo.charm", env), tc.Equals, "")

	// From the same directory, but with HOME elsewhere, the relative path is
	// unreachable and the hint fires.
	elsewhere := utils.SnapEnv{Snap: "/snap/juju/current", Home: c.MkDir()}
	c.Check(utils.SnapConfinementHint("./foo.charm", elsewhere), tc.Not(tc.Equals), "")
}

// TestSnapConfinementHintMessage locks down the exact wording of the hint, so
// that a stray whitespace or wording change is caught here rather than in a
// user's terminal.
func (s *snapHintSuite) TestSnapConfinementHintMessage(c *tc.C) {
	hint := utils.SnapConfinementHint("/tmp/foo.charm", utils.SnapEnv{
		Snap: "/snap/juju/current",
		Home: "/home/user",
	})
	c.Check(hint, tc.Equals, `

The Juju snap is strictly confined and cannot access files outside your home
directory. Move the file into your home directory and try again, for example:

    cp /tmp/foo.charm ~/`)
}

// TestSnapConfinementHintQuotesPaths checks that the suggested cp command is
// safe to paste into a shell: a plain path is left unquoted for readability,
// while a path containing whitespace, shell metacharacters or a single quote is
// quoted so the shell treats it as one literal word.
func (s *snapHintSuite) TestSnapConfinementHintQuotesPaths(c *tc.C) {
	tests := []struct {
		desc string
		path string
		want string
	}{{
		desc: "plain path left unquoted",
		path: "/tmp/foo.charm",
		want: "    cp /tmp/foo.charm ~/",
	}, {
		desc: "path with a space is quoted",
		path: "/tmp/my charm.charm",
		want: "    cp '/tmp/my charm.charm' ~/",
	}, {
		desc: "shell metacharacters are not expanded",
		path: "/tmp/a$b`c.charm",
		want: "    cp '/tmp/a$b`c.charm' ~/",
	}, {
		desc: "embedded single quote is escaped",
		path: "/tmp/it's.charm",
		want: `    cp '/tmp/it'\''s.charm' ~/`,
	}}

	for _, tt := range tests {
		c.Logf("case: %s", tt.desc)
		hint := utils.SnapConfinementHint(tt.path, utils.SnapEnv{
			Snap:     "/snap/juju/current",
			RealHome: "/home/user",
			Home:     "/home/user",
		})
		c.Check(hint, tc.HasSuffix, tt.want)
	}
}

// TestAnnotateWithSnapHintAppendsHint checks that the hint is appended after
// the original message (rather than prefixed, as a juju/errors annotation
// would be) and that the error's cause and wrap chain survive, so callers can
// still classify the failure.
func (s *snapHintSuite) TestAnnotateWithSnapHintAppendsHint(c *tc.C) {
	c.Setenv("SNAP", "/snap/juju/current")
	c.Setenv("HOME", "/home/user")
	c.Setenv("SNAP_REAL_HOME", "")
	c.Setenv("SNAP_USER_DATA", "")
	c.Setenv("SNAP_USER_COMMON", "")

	cause := errors.Annotatef(os.ErrNotExist, "file for resource %q", "foo")
	err := utils.AnnotateWithSnapHint(cause, "/tmp/foo.charm")

	c.Check(err.Error(), tc.Equals, `file for resource "foo": file does not exist

The Juju snap is strictly confined and cannot access files outside your home
directory. Move the file into your home directory and try again, for example:

    cp /tmp/foo.charm ~/`)
	c.Check(err.Error(), tc.HasPrefix, cause.Error(),
		tc.Commentf("hint should be appended, not prefixed"))
	c.Check(os.IsNotExist(errors.Cause(err)), tc.Equals, true)
	c.Check(errors.Is(err, os.ErrNotExist), tc.Equals, true)
}

// TestAnnotateWithSnapHintNoHint checks that when no hint applies the error is
// returned untouched, so no call site pays for the check.
func (s *snapHintSuite) TestAnnotateWithSnapHintNoHint(c *tc.C) {
	c.Setenv("SNAP", "")

	cause := errors.Annotatef(os.ErrNotExist, "file for resource %q", "foo")
	c.Check(utils.AnnotateWithSnapHint(cause, "/tmp/foo.charm"), tc.Equals, cause)
}

// TestAnnotateWithSnapHintNilError checks the nil-in, nil-out contract shared
// by the juju/errors annotate helpers.
func (s *snapHintSuite) TestAnnotateWithSnapHintNilError(c *tc.C) {
	c.Setenv("SNAP", "/snap/juju/current")
	c.Check(utils.AnnotateWithSnapHint(nil, "/tmp/foo.charm"), tc.IsNil)
}
