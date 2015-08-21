// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions_test

import (
	gc "gopkg.in/check.v1"
)

type actionsSuite struct{}

var _ = gc.Suite(&actionsSuite{})

func (s *actionsSuite) TestOne(c *gc.C) {
	c.Assert(42, gc.Equals, 43)
}
