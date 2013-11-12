// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
)

type DirectorySuite struct {
	testbase.LoggingSuite
	containerDir string
	removedDir   string
}

var _ = gc.Suite(&DirectorySuite{})

func (s *DirectorySuite) SetUpTest(c *gc.C) {
	s.LoggingSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.containerDir)
	s.removedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.removedDir)
}

func (*DirectorySuite) TestNewContainerDir(c *gc.C) {
	dir, err := container.NewContainerDirectory("testing")
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDir(c *gc.C) {
	dir, err := container.NewContainerDirectory("testing")
	c.Assert(err, gc.IsNil)
	err = container.RemoveContainerDirectory("testing")
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing"), jc.IsDirectory)
}
