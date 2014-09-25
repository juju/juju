// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
)

type ConstantsSuite struct{}

var _ = gc.Suite(&ConstantsSuite{})

func (s *ConstantsSuite) TestAnyJobNeedsState(c *gc.C) {
	c.Assert(params.AnyJobNeedsState(), jc.IsFalse)
	c.Assert(params.AnyJobNeedsState(params.JobHostUnits), jc.IsFalse)
	c.Assert(params.AnyJobNeedsState(params.JobManageNetworking), jc.IsFalse)
	c.Assert(params.AnyJobNeedsState(params.JobManageStateDeprecated), jc.IsFalse)
	c.Assert(params.AnyJobNeedsState(params.JobManageEnviron), jc.IsTrue)
	c.Assert(params.AnyJobNeedsState(params.JobHostUnits, params.JobManageEnviron), jc.IsTrue)
}
