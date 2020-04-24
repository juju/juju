// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package featureflag_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	"github.com/juju/worker/v2/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/worker/featureflag"
)

type ValidationSuite struct {
	testing.IsolationSuite
	config featureflag.ManifoldConfig
}

var _ = gc.Suite(&ValidationSuite{})

func (s *ValidationSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config.StateName = "state"
	s.config.FlagName = "new-hotness"
	s.config.NewWorker = func(featureflag.Config) (worker.Worker, error) {
		return nil, nil
	}
}

func (s *ValidationSuite) TestMissingStateName(c *gc.C) {
	s.config.StateName = ""
	c.Assert(s.config.Validate(), gc.ErrorMatches, "empty StateName not valid")
}

func (s *ValidationSuite) TestMissingFlagName(c *gc.C) {
	s.config.FlagName = ""
	c.Assert(s.config.Validate(), gc.ErrorMatches, "empty FlagName not valid")
}

func (s *ValidationSuite) TestMissingNewWorker(c *gc.C) {
	s.config.NewWorker = nil
	c.Assert(s.config.Validate(), gc.ErrorMatches, "nil NewWorker not valid")
}

type ManifoldSuite struct {
	statetesting.StateSuite

	manifold     dependency.Manifold
	context      dependency.Context
	stateTracker stubStateTracker
	worker       *mockWorker

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.stub.ResetCalls()
	s.worker = &mockWorker{}
	s.stateTracker.ResetCalls()
	s.stateTracker.pool = s.StatePool

	s.context = s.newContext(nil)
	s.manifold = featureflag.Manifold(featureflag.ManifoldConfig{
		StateName: "state",
		FlagName:  "new-hotness",
		Invert:    true,
		NewWorker: s.newWorker,
	})
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"state": &s.stateTracker,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config featureflag.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return s.worker, nil
}

var expectedInputs = []string{"state"}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	s.startWorkerClean(c)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], jc.DeepEquals, featureflag.Config{
		Source:   s.State,
		FlagName: "new-hotness",
		Invert:   true,
	})
}

func (s *ManifoldSuite) TestErrRefresh(c *gc.C) {
	w := s.startWorkerClean(c)

	s.worker.SetErrors(featureflag.ErrRefresh)
	err := w.Wait()
	c.Assert(s.manifold.Filter(err), gc.Equals, dependency.ErrBounce)
}

func (s *ManifoldSuite) TestOutput(c *gc.C) {
	s.worker.result = true
	w := s.startWorkerClean(c)

	var flag engine.Flag
	err := s.manifold.Output(w, &flag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(flag.Check(), jc.IsTrue)
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) TestClosesStateOnWorkerError(c *gc.C) {
	s.stub.SetErrors(errors.Errorf("splat"))
	w, err := s.manifold.Start(s.context)
	c.Assert(err, gc.ErrorMatches, "splat")
	c.Assert(w, gc.IsNil)

	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

type mockWorker struct {
	testing.Stub
	result bool
}

func (w *mockWorker) Check() bool {
	return w.result
}

func (w *mockWorker) Kill() {}

func (w *mockWorker) Wait() error {
	return w.NextErr()
}

type stubStateTracker struct {
	testing.Stub
	pool *state.StatePool
}

func (s *stubStateTracker) Use() (*state.StatePool, error) {
	s.MethodCall(s, "Use")
	return s.pool, s.NextErr()
}

func (s *stubStateTracker) Done() error {
	s.MethodCall(s, "Done")
	return s.NextErr()
}

func (s *stubStateTracker) Report() map[string]interface{} {
	s.MethodCall(s, "Report")
	return nil
}
