// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"os"
	"runtime"

	"github.com/juju/tc"
	"github.com/juju/testing"
)

type profileSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&profileSuite{})

func (*profileSuite) TestProfileFilename(c *tc.C) {
	c.Assert(profileFilename(ProfileDir), tc.Equals, "/etc/profile.d/juju-introspection.sh")
}

func (*profileSuite) TestNonLinux(c *tc.C) {
	if runtime.GOOS == "linux" {
		c.Skip("testing non-linux")
	}
	err := WriteProfileFunctions(ProfileDir)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *profileSuite) TestLinux(c *tc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("testing linux")
	}
	dir := c.MkDir()
	err := WriteProfileFunctions(dir)
	c.Assert(err, tc.ErrorIsNil)

	content, err := os.ReadFile(profileFilename(dir))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(content), tc.Equals, shellFuncs)
}
