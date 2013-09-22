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
	"launchpad.net/juju-core/testing/testbase"
)

// TestSuite replaces the lxc factory that the broker uses with a mock
// implementation.
type TestSuite struct {
	testbase.LoggingSuite
	Factory      mock.ContainerFactory
	ContainerDir string
	RemovedDir   string
	LxcDir       string
	RestartDir   string
}

func (s *TestSuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.ContainerDir = c.MkDir()
	s.PatchValue(&containerDir, s.ContainerDir)
	s.RemovedDir = c.MkDir()
	s.PatchValue(&removedContainerDir, s.RemovedDir)
	s.LxcDir = c.MkDir()
	s.PatchValue(&lxcContainerDir, s.LxcDir)
	s.RestartDir = c.MkDir()
	s.PatchValue(&lxcRestartDir, s.RestartDir)
	s.Factory = mock.MockFactory()
	s.PatchValue(&lxcObjectFactory, s.Factory)
}
