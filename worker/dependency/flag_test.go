// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
)

type FlagSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FlagSuite{})

func (*FlagSuite) TestEmptyInputs(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "blob")
	c.Check(wrapped.Inputs, jc.DeepEquals, []string{"blob"})
}

func (*FlagSuite) TestNonEmptyInputs(c *gc.C) {
	base := dependency.Manifold{
		Inputs: []string{"foo", "bar"},
	}
	wrapped := dependency.WithFlag(base, "blib")
	expect := []string{"foo", "bar", "blib"}
	c.Check(wrapped.Inputs, jc.DeepEquals, expect)
}

func (*FlagSuite) TestEmptyOutput(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "blob")
	c.Check(wrapped.Output, gc.IsNil)
}

func (*FlagSuite) TestNonEmptyOutput(c *gc.C) {
	output := func(_ worker.Worker, _ interface{}) error {
		panic("splat")
	}
	base := dependency.Manifold{
		Output: output,
	}
	wrapped := dependency.WithFlag(base, "blah")
	tryOutput := func() {
		wrapped.Output(nil, nil)
	}
	c.Check(tryOutput, gc.PanicMatches, "splat")
}

func (*FlagSuite) TestEmptyFilter(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "blob")
	c.Check(wrapped.Filter, gc.IsNil)
}

func (*FlagSuite) TestNonEmptyFilter(c *gc.C) {
	filter := func(err error) error {
		panic(err)
	}
	base := dependency.Manifold{
		Filter: filter,
	}
	wrapped := dependency.WithFlag(base, "blah")
	tryFilter := func() {
		wrapped.Filter(errors.New("splat"))
	}
	c.Check(tryFilter, gc.PanicMatches, "splat")
}

func (*FlagSuite) TestStartMissingFlag(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "foo")
	context := dt.StubContext(nil, map[string]interface{}{
		"foo": dependency.ErrMissing,
	})
	worker, err := wrapped.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*FlagSuite) TestStartNotFlag(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "foo")
	context := dt.StubContext(nil, map[string]interface{}{
		"foo": true,
	})
	worker, err := wrapped.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `cannot set true into \*dependency.Flag`)
}

func (*FlagSuite) TestStartFalseFlag(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "foo")
	context := dt.StubContext(nil, map[string]interface{}{
		"foo": stubFlag(false),
	})
	worker, err := wrapped.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (*FlagSuite) TestStartTrueFlag(c *gc.C) {
	expectWorker := &stubWorker{}
	base := dependency.Manifold{
		Start: func(_ dependency.Context) (worker.Worker, error) {
			return expectWorker, nil
		},
	}
	wrapped := dependency.WithFlag(base, "foo")
	context := dt.StubContext(nil, map[string]interface{}{
		"foo": stubFlag(true),
	})
	worker, err := wrapped.Start(context)
	c.Check(worker, gc.Equals, expectWorker)
	c.Check(err, jc.ErrorIsNil)
}

func (*FlagSuite) TestFlagOutputBadWorker(c *gc.C) {
	in := &stubWorker{}
	var out dependency.Flag
	err := dependency.FlagOutput(in, &out)
	c.Check(err, gc.ErrorMatches, `expected in to implement Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestFlagOutputBadTarget(c *gc.C) {
	in := &stubFlagWorker{}
	var out interface{}
	err := dependency.FlagOutput(in, &out)
	c.Check(err, gc.ErrorMatches, `expected out to be a \*Flag; got a .*`)
	c.Check(out, gc.IsNil)
}

func (*FlagSuite) TestFlagOutputSuccess(c *gc.C) {
	in := &stubFlagWorker{}
	var out dependency.Flag
	err := dependency.FlagOutput(in, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out, gc.Equals, in)
}

type stubFlag bool

func (flag stubFlag) Check() bool {
	return bool(flag)
}

type stubWorker struct {
	worker.Worker
}

type stubFlagWorker struct {
	dependency.Flag
	worker.Worker
}
