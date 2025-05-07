// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/model"
)

type ConstantsSuite struct{}

var _ = tc.Suite(&ConstantsSuite{})

func (s *ConstantsSuite) TestAnyJobNeedsState(c *tc.C) {
	c.Assert(model.AnyJobNeedsState(), tc.IsFalse)
	c.Assert(model.AnyJobNeedsState(model.JobHostUnits), tc.IsFalse)
	c.Assert(model.AnyJobNeedsState(model.JobManageModel), tc.IsTrue)
	c.Assert(model.AnyJobNeedsState(model.JobHostUnits, model.JobManageModel), tc.IsTrue)
}
