// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"os"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/container"
)

type DirectorySuite struct {
	testing.CleanupSuite
	containerDir string
	removedDir   string
}

var _ = gc.Suite(&DirectorySuite{})

func (s *DirectorySuite) SetUpTest(c *gc.C) {
	s.CleanupSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.containerDir)
	s.removedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.removedDir)
}

func (*DirectorySuite) TestNewContainerDir(c *gc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDir(c *gc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	err = container.RemoveDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir, jc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing"), jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDirWithClash(c *gc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)

	clash := filepath.Join(s.removedDir, "testing")
	err = os.MkdirAll(clash, 0755)
	c.Assert(err, jc.ErrorIsNil)

	err = container.RemoveDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir, jc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing.1"), jc.IsDirectory)
}
