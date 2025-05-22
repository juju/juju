// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package container_test

import (
	"os"
	"path/filepath"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/container"
	"github.com/juju/juju/internal/testing"
)

type DirectorySuite struct {
	testing.BaseSuite
	containerDir string
	removedDir   string
}

func TestDirectorySuite(t *stdtesting.T) {
	tc.Run(t, &DirectorySuite{})
}

func (s *DirectorySuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.containerDir = c.MkDir()
	s.PatchValue(&container.ContainerDir, s.containerDir)
	s.removedDir = c.MkDir()
	s.PatchValue(&container.RemovedContainerDir, s.removedDir)
}

func (*DirectorySuite) TestNewContainerDir(c *tc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir, tc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDir(c *tc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, tc.ErrorIsNil)
	err = container.RemoveDirectory("testing")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir, tc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing"), tc.IsDirectory)
}

func (s *DirectorySuite) TestRemoveContainerDirWithClash(c *tc.C) {
	dir, err := container.NewDirectory("testing")
	c.Assert(err, tc.ErrorIsNil)

	clash := filepath.Join(s.removedDir, "testing")
	err = os.MkdirAll(clash, 0755)
	c.Assert(err, tc.ErrorIsNil)

	err = container.RemoveDirectory("testing")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(dir, tc.DoesNotExist)
	c.Assert(filepath.Join(s.removedDir, "testing.1"), tc.IsDirectory)
}
