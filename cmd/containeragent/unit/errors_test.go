// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/cmd/containeragent/unit"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type ErrorsSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestLifeFilter_Nil(c *tc.C) {
	result := unit.LifeFilter(nil)
	c.Check(result, tc.ErrorIsNil)
}

func (*ErrorsSuite) TestLifeFilter_Random(c *tc.C) {
	err := errors.New("whatever")
	result := unit.LifeFilter(err)
	c.Check(result, tc.Equals, err)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Exact(c *tc.C) {
	err := lifeflag.ErrValueChanged
	result := unit.LifeFilter(err)
	c.Check(result, tc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Traced(c *tc.C) {
	err := errors.Trace(lifeflag.ErrValueChanged)
	result := unit.LifeFilter(err)
	c.Check(result, tc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Exact(c *tc.C) {
	err := lifeflag.ErrNotFound
	result := unit.LifeFilter(err)
	c.Check(result, tc.Equals, unit.ErrRemoved)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Traced(c *tc.C) {
	err := errors.Trace(lifeflag.ErrNotFound)
	result := unit.LifeFilter(err)
	c.Check(result, tc.Equals, unit.ErrRemoved)
}
