// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
)

type formatSuite struct {
	testing.LoggingSuite
}

var _ = gc.Suite(&formatSuite{})

func (*formatSuite) TestReadFormat(c *gc.C) {
	format, err := readFormat("ignored")
	c.Assert(format, gc.Equals, currentFormat)
	c.Assert(err, gc.IsNil)
}

func (*formatSuite) TestNewFormatter(c *gc.C) {
	formatter, err := newFormatter(currentFormat)
	c.Assert(formatter, gc.NotNil)
	c.Assert(err, gc.IsNil)

	formatter, err = newFormatter("other")
	c.Assert(formatter, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "unknown agent config format")
}
