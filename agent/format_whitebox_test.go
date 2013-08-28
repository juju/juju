// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The format tests are white box tests, meaning that the tests are in the
// same package as the code, as all the format details are internal to the
// package.

package agent

import (
	"io/ioutil"
	"path"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type formatSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&formatSuite{})

func (*formatSuite) TestReadFormatEmptyDir(c *gc.C) {
	// Since the previous format didn't have a format file, a missing format
	// should return the previous format.  Once we are over the hump of
	// missing format files, a missing format file should generate an error.
	dir := c.MkDir()
	format, err := readFormat(dir)
	c.Assert(format, gc.Equals, previousFormat)
	c.Assert(err, gc.IsNil)
}

func (*formatSuite) TestReadFormat(c *gc.C) {
	dir := c.MkDir()
	err := ioutil.WriteFile(path.Join(dir, formatFilename), []byte("some format\n"), 0644)
	c.Assert(err, gc.IsNil)
	format, err := readFormat(dir)
	c.Assert(format, gc.Equals, "some format")
	c.Assert(err, gc.IsNil)
}

func (*formatSuite) TestNewFormatter(c *gc.C) {
	formatter, err := newFormatter(currentFormat)
	c.Assert(formatter, gc.NotNil)
	c.Assert(err, gc.IsNil)

	formatter, err = newFormatter(previousFormat)
	c.Assert(formatter, gc.NotNil)
	c.Assert(err, gc.IsNil)

	formatter, err = newFormatter("other")
	c.Assert(formatter, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "unknown agent config format")
}
