// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/container/lxc/mock"
	"launchpad.net/juju-core/testing"
)

// TestSuite replaces the lxc factory that the broker uses with a mock
// implementation.
type TestSuite struct {
	testing.FakeJujuHomeSuite
	Factory      mock.ContainerFactory
	ContainerDir string
	RemovedDir   string
	LxcDir       string
	RestartDir   string
}

func (s *TestSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ContainerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.ContainerDir)
	s.RemovedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.RemovedDir)
	s.LxcDir = c.MkDir()
	s.PatchValue(&lxc.LxcContainerDir, s.LxcDir)
	s.RestartDir = c.MkDir()
	s.PatchValue(&lxc.LxcRestartDir, s.RestartDir)
	s.Factory = mock.MockFactory(s.LxcDir)
	s.PatchValue(&lxc.LxcObjectFactory, s.Factory)
}
