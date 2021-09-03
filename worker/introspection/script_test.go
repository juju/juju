// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"io/ioutil"
	"runtime"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type profileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&profileSuite{})

func (*profileSuite) TestProfileFilename(c *gc.C) {
	c.Assert(profileFilename(ProfileDir), gc.Equals, "/etc/profile.d/juju-introspection.sh")
}

func (*profileSuite) TestNonLinux(c *gc.C) {
	if runtime.GOOS == "linux" {
		c.Skip("testing non-linux")
	}
	err := WriteProfileFunctions(ProfileDir)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *profileSuite) TestLinux(c *gc.C) {
	if runtime.GOOS != "linux" {
		c.Skip("testing linux")
	}
	dir := c.MkDir()
	err := WriteProfileFunctions(dir)
	c.Assert(err, jc.ErrorIsNil)

	content, err := ioutil.ReadFile(profileFilename(dir))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(content), gc.Equals, shellFuncs)
}
