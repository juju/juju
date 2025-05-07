// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
)

type seriesSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&seriesSuite{})

func (s *seriesSuite) TestDefaultSupportedLTSBase(c *tc.C) {
	b := DefaultSupportedLTSBase()
	c.Assert(b.String(), tc.Equals, "ubuntu@24.04/stable")
}
