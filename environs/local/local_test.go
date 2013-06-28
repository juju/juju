// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	stdtesting "testing"

	. "launchpad.net/gocheck"
	"launchpad.net/golxc"
	"launchpad.net/juju-core/container/lxc"
	"launchpad.net/juju-core/container/lxc/mock"
	"launchpad.net/juju-core/testing"
)

func TestLocal(t *stdtesting.T) {
	TestingT(t)
}

type MockLxcSuite struct {
	testing.LoggingSuite
	golxc              golxc.ContainerFactory
	containerDir       string
	removedDir         string
	lxcDir             string
	oldContainerDir    string
	oldRemovedDir      string
	oldLxcContainerDir string
}

var _ = Suite(&MockLxcSuite{})

func (s *MockLxcSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *MockLxcSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *MockLxcSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.oldContainerDir = lxc.SetContainerDir(s.containerDir)
	s.removedDir = c.MkDir()
	s.oldRemovedDir = lxc.SetRemovedContainerDir(s.removedDir)
	s.lxcDir = c.MkDir()
	s.oldLxcContainerDir = lxc.SetLxcContainerDir(s.lxcDir)
	s.golxc = mock.MockFactory()
}

func (s *MockLxcSuite) TearDownTest(c *C) {
	lxc.SetContainerDir(s.oldContainerDir)
	lxc.SetLxcContainerDir(s.oldLxcContainerDir)
	lxc.SetRemovedContainerDir(s.oldRemovedDir)
	s.LoggingSuite.TearDownTest(c)
}
