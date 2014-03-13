// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

func Test(t *stdtesting.T) {
	gc.TestingT(t)
}

type InitialiserSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&InitialiserSuite{})

func (s *InitialiserSuite) TestTargetReleasePackages(c *gc.C) {
	cmdChan := s.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	container := NewContainerInitialiser("target")
	err := container.Initialise()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "--target-release", "target", "lxc",
	})
}

func (s *InitialiserSuite) TestNoTargetReleasePackages(c *gc.C) {
	cmdChan := s.HookCommandOutput(&utils.AptCommandOutput, []byte{}, nil)
	container := NewContainerInitialiser("")
	err := container.Initialise()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "lxc",
	})
}
