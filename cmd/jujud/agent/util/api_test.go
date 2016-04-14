// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/cmd/jujud/agent/util"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
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
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ApiManifoldSuite) TestStartFailure(c *gc.C) {
	expectApiCaller := &dummyApiCaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller-name": expectApiCaller,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(context)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "some error")
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectApiCaller},
	}})
}

func (s *ApiManifoldSuite) TestStartSuccess(c *gc.C) {
	expectApiCaller := &dummyApiCaller{}
	context := dt.StubContext(nil, map[string]interface{}{
		"api-caller-name": expectApiCaller,
	})

	worker, err := s.manifold.Start(context)
	c.Check(err, jc.ErrorIsNil)
	c.Check(worker, gc.Equals, s.worker)
	s.CheckCalls(c, []testing.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectApiCaller},
	}})
}
