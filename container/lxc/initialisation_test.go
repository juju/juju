// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxc

import (
	"github.com/juju/utils/apt"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type InitialiserSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&InitialiserSuite{})

func (s *InitialiserSuite) TestLTSSeriesPackages(c *gc.C) {
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	container := NewContainerInitialiser("precise")
	err := container.Initialise()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "--target-release", "precise-updates/cloud-tools", "lxc", "cloud-image-utils",
	})
}

func (s *InitialiserSuite) TestNoSeriesPackages(c *gc.C) {
	cmdChan := s.HookCommandOutput(&apt.CommandOutput, []byte{}, nil)
	container := NewContainerInitialiser("")
	err := container.Initialise()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, []string{
		"apt-get", "--option=Dpkg::Options::=--force-confold",
		"--option=Dpkg::options::=--force-unsafe-io", "--assume-yes", "--quiet",
		"install", "lxc", "cloud-image-utils",
	})
}
