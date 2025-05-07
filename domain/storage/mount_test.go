// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/storage"
)

type mountSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&mountSuite{})

func (s *mountSuite) TestMountPointSingleton(c *gc.C) {
	path, err := storage.FilesystemMountPoint("here", 1, "data/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "here")
}

func (s *mountSuite) TestMountPointLocation(c *gc.C) {
	path, err := storage.FilesystemMountPoint("/path/to/here", 2, "data/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "/path/to/here/data/0")
}

func (s *mountSuite) TestMountPointNoLocation(c *gc.C) {
	path, err := storage.FilesystemMountPoint("", 2, "data/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(path, gc.Equals, "/var/lib/juju/storage/data/0")
}

func (s *mountSuite) TestBadMountPoint(c *gc.C) {
	_, err := storage.FilesystemMountPoint("/var/lib/juju/storage/here", 1, "data/0")
	c.Assert(err, gc.ErrorMatches, "invalid location .*")
}
