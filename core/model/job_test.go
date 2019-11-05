// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/model"
)

type ConstantsSuite struct{}

var _ = gc.Suite(&ConstantsSuite{})

func (s *ConstantsSuite) TestAnyJobNeedsState(c *gc.C) {
	c.Assert(model.AnyJobNeedsState(), jc.IsFalse)
	c.Assert(model.AnyJobNeedsState(model.JobHostUnits), jc.IsFalse)
	c.Assert(model.AnyJobNeedsState(model.JobManageModel), jc.IsTrue)
	c.Assert(model.AnyJobNeedsState(model.JobHostUnits, model.JobManageModel), jc.IsTrue)
}
