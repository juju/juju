// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
	jc "launchpad.net/juju-core/testing/checkers"
)


type limiterSuite struct {}

var _ = gc.Suite(&limiterSuite{})

func (*limiterSuite) TestAcquireUntilFull(c *gc.C) {
	l := utils.NewLimiter(2)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsFalse)
}

func (*limiterSuite) TestBadRelease(c *gc.C) {
	l := utils.NewLimiter(2)
	c.Check(l.Release(), gc.ErrorMatches, "Release without an associated Acquire")
}

func (*limiterSuite) TestAcquireAndRelease(c *gc.C) {
	l := utils.NewLimiter(2)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Acquire(), jc.IsFalse)
	c.Check(l.Release(), gc.IsNil)
	c.Check(l.Acquire(), jc.IsTrue)
	c.Check(l.Release(), gc.IsNil)
	c.Check(l.Release(), gc.IsNil)
	c.Check(l.Release(), gc.ErrorMatches, "Release without an associated Acquire")
}
