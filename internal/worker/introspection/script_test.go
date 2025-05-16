// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"os"
	"runtime"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type profileSuite struct {
	testhelpers.IsolationSuite
}

func TestProfileSuite(t *stdtesting.T) { tc.Run(t, &profileSuite{}) }
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
