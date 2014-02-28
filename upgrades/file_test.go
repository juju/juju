// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upgrades"
)

type fileSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&fileSuite{})

func checkFile(c *gc.C, filename string) {
	context, err := ioutil.ReadFile(filename)
	c.Assert(string(context), gc.Equals, "world")
	fi, err := os.Stat(filename)
	c.Assert(err, gc.IsNil)
	mode := os.FileMode(0644)
	c.Assert(fi.Mode()&(os.ModeType|mode), gc.Equals, mode)
}

func (s *fileSuite) TestWriteReplacementFileWhenNotExists(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "foo.conf")
	err := upgrades.WriteReplacementFile(filename, []byte("world"), 0644)
	c.Assert(err, gc.IsNil)
	checkFile(c, filename)
}

func (s *fileSuite) TestWriteReplacementFileWhenExists(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "foo.conf")
	err := ioutil.WriteFile(filename, []byte("hello"), 0755)
	c.Assert(err, gc.IsNil)
	err = upgrades.WriteReplacementFile(filename, []byte("world"), 0644)
	c.Assert(err, gc.IsNil)
	checkFile(c, filename)
}
