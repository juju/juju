// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	gc "gopkg.in/check.v1"
)

type FactorySuite struct{}

var _ = gc.Suite(&FactorySuite{})

func (s *FactorySuite) TestFatal(c *gc.C) {
	c.Fatalf("GFY")
}
