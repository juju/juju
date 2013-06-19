// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type FileSuite struct{}

var _ = Suite(&FileSuite{})

func (s *FileSuite) AssertDirectoryExistsOrNot(c *C) {
	dir := c.MkDir()
	testing.AssertDirectoryExists(c, dir)

	absentDir := filepath.Join(dir, "foo")
	testing.AssertDirectoryDoesNotExist(c, absentDir)
}
