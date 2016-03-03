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

func (s *FlagSuite) TestEmptyInputs(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "blob")
	c.Check(wrapped.Inputs, jc.DeepEquals, []string{"blob"})
}

func (s *FlagSuite) TestNonEmptyInputs(c *gc.C) {
	base := dependency.Manifold{
		Inputs: []string{"foo", "bar"},
	}
	wrapped := dependency.WithFlag(base, "blib")
	expect := []string{"foo", "bar", "blib"}
	c.Check(wrapped.Inputs, jc.DeepEquals, expect)
}

func (s *FlagSuite) TestEmptyOutput(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "blob")
	c.Check(wrapped.Output, gc.IsNil)
}

func (s *FlagSuite) TestNonEmptyOutput(c *gc.C) {
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

func (s *FlagSuite) TestStartMissingFlag(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "foo")
	getResource := dt.StubGetResource(dt.StubResources{
		"foo": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := wrapped.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *FlagSuite) TestStartNotFlag(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "foo")
	getResource := dt.StubGetResource(dt.StubResources{
		"foo": dt.StubResource{Output: true},
	})
	worker, err := wrapped.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `cannot set true into \*dependency.Flag`)
}

func (s *FlagSuite) TestStartFalseFlag(c *gc.C) {
	wrapped := dependency.WithFlag(dependency.Manifold{}, "foo")
	getResource := dt.StubGetResource(dt.StubResources{
		"foo": dt.StubResource{Output: stubFlag(false)},
	})
	worker, err := wrapped.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
}

func (s *FlagSuite) TestStartTrueFlag(c *gc.C) {
	expectWorker := &stubWorker{}
	base := dependency.Manifold{
		Start: func(_ dependency.GetResourceFunc) (worker.Worker, error) {
			return expectWorker, nil
		},
	}
	wrapped := dependency.WithFlag(base, "foo")
	getResource := dt.StubGetResource(dt.StubResources{
		"foo": dt.StubResource{Output: stubFlag(true)},
	})
	worker, err := wrapped.Start(getResource)
	c.Check(worker, gc.Equals, expectWorker)
	c.Check(err, jc.ErrorIsNil)
}

type stubFlag bool

func (flag stubFlag) Check() bool {
	return bool(flag)
}

type stubWorker struct {
	worker.Worker
}
