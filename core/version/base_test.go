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

func (s *seriesSuite) TestDefaultSupportedLTSBase(c *gc.C) {
	b := DefaultSupportedLTSBase()
	c.Assert(b.String(), gc.Equals, "ubuntu@24.04/stable")
}
