// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
)

type FileSuite struct{}

var _ = Suite(&FileSuite{})

func (s *FileSuite) TestAssertDirectoryExistsOrNot(c *C) {
	dir := c.MkDir()
	testing.AssertDirectoryExists(c, dir)

	absentDir := filepath.Join(dir, "foo")
	testing.AssertDirectoryDoesNotExist(c, absentDir)
}

func (s *FileSuite) TestAssertNonEmptyFileExists(c *C) {
	file, err := ioutil.TempFile(c.MkDir(), "")
	c.Assert(err, IsNil)
	fmt.Fprintf(file, "something")
	file.Close()

	testing.AssertNonEmptyFileExists(c, file.Name())
}
