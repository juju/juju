// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine_test

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/testhelpers"
)

type APIManifoldSuite struct {
	testhelpers.IsolationSuite
	testhelpers.Stub
	manifold dependency.Manifold
	worker   worker.Worker
}

func TestAPIManifoldSuite(t *testing.T) {
	tc.Run(t, &APIManifoldSuite{})
}

func (s *APIManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.Stub = testhelpers.Stub{}
	s.worker = &dummyWorker{}
	s.manifold = engine.APIManifold(engine.APIManifoldConfig{
		APICallerName: "api-caller-name",
	}, s.newWorker)
}

func (s *APIManifoldSuite) newWorker(apiCaller base.APICaller) (worker.Worker, error) {
	s.AddCall("newWorker", apiCaller)
	if err := s.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

func (s *APIManifoldSuite) TestInputs(c *tc.C) {
	c.Check(s.manifold.Inputs, tc.DeepEquals, []string{"api-caller-name"})
}

func (s *APIManifoldSuite) TestOutput(c *tc.C) {
	c.Check(s.manifold.Output, tc.IsNil)
}

func (s *APIManifoldSuite) TestStartAPIMissing(c *tc.C) {
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller-name": dependency.ErrMissing,
	})

	worker, err := s.manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.Equals, dependency.ErrMissing)
}

func (s *APIManifoldSuite) TestStartFailure(c *tc.C) {
	expectAPICaller := &dummyAPICaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller-name": expectAPICaller,
	})
	s.SetErrors(errors.New("some error"))

	worker, err := s.manifold.Start(c.Context(), getter)
	c.Check(worker, tc.IsNil)
	c.Check(err, tc.ErrorMatches, "some error")
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAPICaller},
	}})
}

func (s *APIManifoldSuite) TestStartSuccess(c *tc.C) {
	expectAPICaller := &dummyAPICaller{}
	getter := dt.StubGetter(map[string]interface{}{
		"api-caller-name": expectAPICaller,
	})

	worker, err := s.manifold.Start(c.Context(), getter)
	c.Check(err, tc.ErrorIsNil)
	c.Check(worker, tc.Equals, s.worker)
	s.CheckCalls(c, []testhelpers.StubCall{{
		FuncName: "newWorker",
		Args:     []interface{}{expectAPICaller},
	}})
}
