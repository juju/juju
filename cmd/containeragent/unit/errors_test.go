// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unit_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/containeragent/unit"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type ErrorsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestLifeFilter_Nil(c *gc.C) {
	result := unit.LifeFilter(nil)
	c.Check(result, jc.ErrorIsNil)
}

func (*ErrorsSuite) TestLifeFilter_Random(c *gc.C) {
	err := errors.New("whatever")
	result := unit.LifeFilter(err)
	c.Check(result, gc.Equals, err)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Exact(c *gc.C) {
	err := lifeflag.ErrValueChanged
	result := unit.LifeFilter(err)
	c.Check(result, gc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Traced(c *gc.C) {
	err := errors.Trace(lifeflag.ErrValueChanged)
	result := unit.LifeFilter(err)
	c.Check(result, gc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Exact(c *gc.C) {
	err := lifeflag.ErrNotFound
	result := unit.LifeFilter(err)
	c.Check(result, gc.Equals, unit.ErrRemoved)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Traced(c *gc.C) {
	err := errors.Trace(lifeflag.ErrNotFound)
	result := unit.LifeFilter(err)
	c.Check(result, gc.Equals, unit.ErrRemoved)
}
