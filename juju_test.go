// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ConstantsSuite struct{}

var _ = gc.Suite(&ConstantsSuite{})

func (s *ConstantsSuite) TestAnyJobNeedsState(c *gc.C) {
	c.Assert(AnyJobNeedsState(), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobHostUnits), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageNetworking), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageStateDeprecated), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageEnviron), jc.IsTrue)
	c.Assert(AnyJobNeedsState(JobHostUnits, JobManageEnviron), jc.IsTrue)
}
