// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type seriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&seriesSuite{})

func (s *seriesSuite) TestDefaultSupportedLTS(c *gc.C) {
	name := DefaultSupportedLTS()
	c.Assert(name, gc.Equals, "bionic")
}
