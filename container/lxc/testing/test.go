// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc"
	"github.com/juju/juju/container/lxc/mock"
	"github.com/juju/juju/testing"
)

// TestSuite replaces the lxc factory that the broker uses with a mock
// implementation.
type TestSuite struct {
	testing.FakeJujuHomeSuite
	ContainerFactory mock.ContainerFactory
	ContainerDir     string
	RemovedDir       string
	LxcDir           string
	RestartDir       string
}

func (s *TestSuite) SetUpTest(c *gc.C) {
	s.FakeJujuHomeSuite.SetUpTest(c)
	s.ContainerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.ContainerDir)
	c.Logf("container.ContainerDir = %q", s.ContainerDir)
	s.RemovedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.RemovedDir)
	c.Logf("container.RemovedContainerDir = %q", s.RemovedDir)
	s.LxcDir = c.MkDir()
	s.PatchValue(&lxc.LxcContainerDir, s.LxcDir)
	c.Logf("lxc.LxcContainerDir = %q", s.LxcDir)
	s.RestartDir = c.MkDir()
	s.PatchValue(&lxc.LxcRestartDir, s.RestartDir)
	c.Logf("lxc.LxcRestartDir = %q", s.RestartDir)
	s.ContainerFactory = mock.MockFactory(s.LxcDir)
	s.PatchValue(&lxc.LxcObjectFactory, s.ContainerFactory)
	c.Logf("lxc.LxcObjectFactory dir = %q", s.LxcDir)
}
