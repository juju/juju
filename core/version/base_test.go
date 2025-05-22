// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package version

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type seriesSuite struct {
	testhelpers.IsolationSuite
}

func TestSeriesSuite(t *testing.T) {
	tc.Run(t, &seriesSuite{})
}

func (s *seriesSuite) TestDefaultSupportedLTSBase(c *tc.C) {
	b := DefaultSupportedLTSBase()
	c.Assert(b.String(), tc.Equals, "ubuntu@24.04/stable")
}
