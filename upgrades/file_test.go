// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "launchpad.net/gocheck"

	"io/ioutil"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upgrades"
	"path/filepath"
)

type fileSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&fileSuite{})

func (s *fileSuite) TestWriteReplacementFileWhenNotExists(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "foo.conf")
	err := upgrades.WriteReplacementFile(filename, []byte("world"))
	c.Assert(err, gc.IsNil)
	context, err := ioutil.ReadFile(filename)
	c.Assert(string(context), gc.Equals, "world")
}

func (s *fileSuite) TestWriteReplacementFileWhenExists(c *gc.C) {
	dir := c.MkDir()
	filename := filepath.Join(dir, "foo.conf")
	err := ioutil.WriteFile(filename, []byte("hello"), 0644)
	c.Assert(err, gc.IsNil)
	err = upgrades.WriteReplacementFile(filename, []byte("world"))
	c.Assert(err, gc.IsNil)
	context, err := ioutil.ReadFile(filename)
	c.Assert(string(context), gc.Equals, "world")
}
