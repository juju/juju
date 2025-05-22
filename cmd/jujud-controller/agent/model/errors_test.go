// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/cmd/jujud-controller/agent/model"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lifeflag"
)

type ErrorsSuite struct {
	testhelpers.IsolationSuite
}

func TestErrorsSuite(t *stdtesting.T) {
	tc.Run(t, &ErrorsSuite{})
}

func (*ErrorsSuite) TestIsFatal_Nil(c *tc.C) {
	result := model.IsFatal(nil)
	c.Check(result, tc.IsFalse)
}

func (*ErrorsSuite) TestIsFatal_Random(c *tc.C) {
	err := errors.New("whatever")
	result := model.IsFatal(err)
	c.Check(result, tc.IsFalse)
}

func (*ErrorsSuite) TestIsFatal_Exact(c *tc.C) {
	err := model.ErrRemoved
	result := model.IsFatal(err)
	c.Check(result, tc.IsTrue)
}

func (*ErrorsSuite) TestIsFatal_Traced(c *tc.C) {
	err := errors.Trace(model.ErrRemoved)
	result := model.IsFatal(err)
	c.Check(result, tc.IsTrue)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Nil(c *tc.C) {
	result := model.IgnoreErrRemoved(nil)
	c.Check(result, tc.ErrorIsNil)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Random(c *tc.C) {
	err := errors.New("whatever")
	result := model.IgnoreErrRemoved(err)
	c.Check(result, tc.Equals, err)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Exact(c *tc.C) {
	err := model.ErrRemoved
	result := model.IgnoreErrRemoved(err)
	c.Check(result, tc.ErrorIsNil)
}

func (*ErrorsSuite) TestIgnoreErrRemoved_Traced(c *tc.C) {
	err := errors.Trace(model.ErrRemoved)
	result := model.IgnoreErrRemoved(err)
	c.Check(result, tc.ErrorIsNil)
}

func (*ErrorsSuite) TestLifeFilter_Nil(c *tc.C) {
	result := model.LifeFilter(nil)
	c.Check(result, tc.ErrorIsNil)
}

func (*ErrorsSuite) TestLifeFilter_Random(c *tc.C) {
	err := errors.New("whatever")
	result := model.LifeFilter(err)
	c.Check(result, tc.Equals, err)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Exact(c *tc.C) {
	err := lifeflag.ErrValueChanged
	result := model.LifeFilter(err)
	c.Check(result, tc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_ValueChanged_Traced(c *tc.C) {
	err := errors.Trace(lifeflag.ErrValueChanged)
	result := model.LifeFilter(err)
	c.Check(result, tc.Equals, dependency.ErrBounce)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Exact(c *tc.C) {
	err := lifeflag.ErrNotFound
	result := model.LifeFilter(err)
	c.Check(result, tc.Equals, model.ErrRemoved)
}

func (*ErrorsSuite) TestLifeFilter_NotFound_Traced(c *tc.C) {
	err := errors.Trace(lifeflag.ErrNotFound)
	result := model.LifeFilter(err)
	c.Check(result, tc.Equals, model.ErrRemoved)
}
