// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/container"
	"launchpad.net/juju-core/testing"
)

type DirectorySuite struct {
	testing.BaseSuite
	containerDir string
	removedDir   string
}

var _ = gc.Suite(&DirectorySuite{})

func (s *DirectorySuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
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
