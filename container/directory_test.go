// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"os"
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
	dir, err := container.NewDirectory("testing")
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDir(c *gc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, gc.IsNil)
	err = container.RemoveDirectory("testing")
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing"), jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDirWithClash(c *gc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, gc.IsNil)

	clash := filepath.Join(s.removedDir, "testing")
	err = os.MkdirAll(clash, 0755)
	c.Assert(err, gc.IsNil)

	err = container.RemoveDirectory("testing")
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing.1"), jc.IsDirectory)
}
