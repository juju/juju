// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/model"
)

type ConstantsSuite struct{}

var _ = tc.Suite(&ConstantsSuite{})

func (s *ConstantsSuite) TestAnyJobNeedsState(c *tc.C) {
	c.Assert(model.AnyJobNeedsState(), jc.IsFalse)
	c.Assert(model.AnyJobNeedsState(model.JobHostUnits), jc.IsFalse)
	c.Assert(model.AnyJobNeedsState(model.JobManageModel), jc.IsTrue)
	c.Assert(model.AnyJobNeedsState(model.JobHostUnits, model.JobManageModel), jc.IsTrue)
}
