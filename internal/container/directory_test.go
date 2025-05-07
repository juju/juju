// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/testing"
)

type DirectorySuite struct {
	testing.BaseSuite
	containerDir string
	removedDir   string
}

var _ = tc.Suite(&DirectorySuite{})

func (s *DirectorySuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.containerDir)
	s.removedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.removedDir)
}

func (*DirectorySuite) TestNewContainerDir(c *tc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDir(c *tc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	err = container.RemoveDirectory("testing")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dir, jc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing"), jc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDirWithClash(c *tc.C) {
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
