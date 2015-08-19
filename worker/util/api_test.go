// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	"github.com/juju/juju/worker/util"
)

type ApiManifoldSuite struct {
	testing.IsolationSuite
	testing.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&ApiManifoldSuite{})

func (s *ApiManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testing.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = util.ApiManifold(util.ApiManifoldConfig{
		APICallerName: "api-caller-name",
	}, s.newWorker)
}

func (s *ApiManifoldSuite) newWorker(apiCaller base.APICaller) (worker.Worker, error) {
	s.AddCall("newWorker", apiCaller)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *ApiManifoldSuite) TestInputs(c *gc.C) {
	c.Check(s.manifold.Inputs, jc.DeepEquals, []string{"api-caller-name"})
}

func (s *ApiManifoldSuite) TestOutput(c *gc.C) {
	c.Check(s.manifold.Output, gc.IsNil)
}

func (s *ApiManifoldSuite) TestStartApiMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller-name": dt.StubResource{Error: dependency.ErrMissing},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ApiManifoldSuite) TestStartFailure(c *gc.C) {
	expectApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller-name": dt.StubResource{Output: expectApiCaller},
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectApiCaller},
	}})
}

func (s *ApiManifoldSuite) TestStartSuccess(c *gc.C) {
	expectApiCaller := &dummyApiCaller{}
	getResource := dt.StubGetResource(dt.StubResources{
		"api-caller-name": dt.StubResource{Output: expectApiCaller},
	})

	worker, err := s.manifold.Start(getResource)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectApiCaller},
	}})
}
