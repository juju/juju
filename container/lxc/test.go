// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Functions defined in this file should *ONLY* be used for testing.  These
// functions are exported for testing purposes only, and shouldn't be called
// from code that isn't in a test file.

package lxc

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/golxc"

	"launchpad.net/juju-core/container/lxc/mock"
)

// SetContainerDir allows tests in other packages to override the
// containerDir.
func SetContainerDir(dir string) (old string) {
	old, containerDir = containerDir, dir
	return
}

// SetLxcContainerDir allows tests in other packages to override the
// lxcContainerDir.
func SetLxcContainerDir(dir string) (old string) {
	old, lxcContainerDir = lxcContainerDir, dir
	return
}

// SetRemovedContainerDir allows tests in other packages to override the
// removedContainerDir.
func SetRemovedContainerDir(dir string) (old string) {
	old, removedContainerDir = removedContainerDir, dir
	return
}

// SetLxcFactory allows tests in other packages to override the lxcObjectFactory
func SetLxcFactory(factory golxc.ContainerFactory) (old golxc.ContainerFactory) {
	logger.Infof("lxcObjectFactory replaced with %v", factory)
	old, lxcObjectFactory = lxcObjectFactory, factory
	return
}

// TestSuite replaces the lxc factory that the broker uses with a mock
// implementation.
type TestSuite struct {
	Factory            mock.ContainerFactory
	oldFactory         golxc.ContainerFactory
	ContainerDir       string
	RemovedDir         string
	LxcDir             string
	oldContainerDir    string
	oldRemovedDir      string
	oldLxcContainerDir string
}

func (s *TestSuite) SetUpSuite(c *gc.C) {}

func (s *TestSuite) TearDownSuite(c *gc.C) {}

func (s *TestSuite) SetUpTest(c *gc.C) {
	s.ContainerDir = c.MkDir()
	s.oldContainerDir = SetContainerDir(s.ContainerDir)
	s.RemovedDir = c.MkDir()
	s.oldRemovedDir = SetRemovedContainerDir(s.RemovedDir)
	s.LxcDir = c.MkDir()
	s.oldLxcContainerDir = SetLxcContainerDir(s.LxcDir)
	s.Factory = mock.MockFactory()
	s.oldFactory = SetLxcFactory(s.Factory)
}

func (s *TestSuite) TearDownTest(c *gc.C) {
	SetContainerDir(s.oldContainerDir)
	SetLxcContainerDir(s.oldLxcContainerDir)
	SetRemovedContainerDir(s.oldRemovedDir)
	SetLxcFactory(s.oldFactory)
}
