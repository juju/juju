// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/dependency"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/model"
	"github.com/juju/juju/worker/lifeflag"
)

type ErrorsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (*ErrorsSuite) TestIsFatal_Nil(c *gc.C) {
	result := model.IsFatal(nil)
	c.Check(result, jc.IsFalse)
}

func (*ErrorsSuite) TestIsFatal_Random(c *gc.C) {
	err := errors.New("whatever")
	result := model.IsFatal(err)
	c.Check(result, jc.IsFalse)
}

func (*ErrorsSuite) TestIsFatal_Exact(c *gc.C) {
	err := model.ErrRemoved
	result := model.IsFatal(err)
	c.Check(result, jc.IsTrue)
}

func (*ErrorsSuite) TestIsFatal_Traced(c *gc.C) {
	err := errors.Trace(model.ErrRemoved)
	result := model.IsFatal(err)
	c.Check(result, jc.IsTrue)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Nil(c *gc.C) {
	result := model.IgnoreErrRemoved(nil)
	c.Check(result, jc.ErrorIsNil)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Random(c *gc.C) {
	err := errors.New("whatever")
	result := model.IgnoreErrRemoved(err)
	c.Check(result, gc.Equals, err)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Exact(c *gc.C) {
	err := model.ErrRemoved
	result := model.IgnoreErrRemoved(err)
	c.Check(result, jc.ErrorIsNil)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Traced(c *gc.C) {
	err := errors.Trace(model.ErrRemoved)
	result := model.IgnoreErrRemoved(err)
	c.Check(result, jc.ErrorIsNil)
}

func (*ErrorsSuite) TestLifeFilter_Nil(c *gc.C) {
	result := model.LifeFilter(nil)
	c.Check(result, jc.ErrorIsNil)
}

func (*ErrorsSuite) TestLifeFilter_Random(c *gc.C) {
	err := errors.New("whatever")
	result := model.LifeFilter(err)
	c.Check(result, gc.Equals, err)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Exact(c *gc.C) {
	err := lifeflag.ErrValueChanged
	result := model.LifeFilter(err)
	c.Check(result, gc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Traced(c *gc.C) {
	err := errors.Trace(lifeflag.ErrValueChanged)
	result := model.LifeFilter(err)
	c.Check(result, gc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Exact(c *gc.C) {
	err := lifeflag.ErrNotFound
	result := model.LifeFilter(err)
	c.Check(result, gc.Equals, model.ErrRemoved)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Traced(c *gc.C) {
	err := errors.Trace(lifeflag.ErrNotFound)
	result := model.LifeFilter(err)
	c.Check(result, gc.Equals, model.ErrRemoved)
}
