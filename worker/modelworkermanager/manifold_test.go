// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/juju/worker.v1/dependency"
	dt "gopkg.in/juju/worker.v1/dependency/testing"
	"gopkg.in/juju/worker.v1/workertest"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/apiserverhttp"
	"github.com/juju/juju/pki"
	pkitest "github.com/juju/juju/pki/test"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/modelworkermanager"
)

type ManifoldSuite struct {
	statetesting.StateSuite

	authority    pki.Authority
	manifold     dependency.Manifold
	context      dependency.Context
	stateTracker stubStateTracker

	stub testing.Stub
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	var err error
	s.authority, err = pkitest.NewTestAuthority()
	c.Assert(err, jc.ErrorIsNil)

	s.StateSuite.SetUpTest(c)

	s.stateTracker = stubStateTracker{pool: s.StatePool}
	s.stub.ResetCalls()

	s.context = s.newContext(nil)
	s.manifold = modelworkermanager.Manifold(modelworkermanager.ManifoldConfig{
		AgentName:      "agent",
		AuthorityName:  "authority",
		StateName:      "state",
		MuxName:        "mux",
		NewWorker:      s.newWorker,
		NewModelWorker: s.newModelWorker,
		Logger:         loggo.GetLogger("test"),
	})
}

var mux = apiserverhttp.NewMux()

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent":     &fakeAgent{},
		"authority": s.authority,
		"mux":       mux,
		"state":     &s.stateTracker}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(config modelworkermanager.Config) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

func (s *ManifoldSuite) newModelWorker(config modelworkermanager.NewModelConfig) (worker.Worker, error) {
	s.stub.MethodCall(s, "NewModelWorker", config)
	if err := s.stub.NextErr(); err != nil {
		return nil, err
	}
	return worker.NewRunner(worker.RunnerParams{}), nil
}

var expectedInputs = []string{"agent", "authority", "mux", "state"}

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
	w := s.startWorkerClean(c)
	workertest.CleanKill(c, w)

	s.stub.CheckCallNames(c, "NewWorker")
	args := s.stub.Calls()[0].Args
	c.Assert(args, gc.HasLen, 1)
	c.Assert(args[0], gc.FitsTypeOf, modelworkermanager.Config{})
	config := args[0].(modelworkermanager.Config)
	config.Authority = s.authority

	c.Assert(config.NewModelWorker, gc.NotNil)
	modelConfig := modelworkermanager.NewModelConfig{
		Authority: s.authority,
		ModelUUID: "foo",
		ModelType: state.ModelTypeIAAS,
	}
	mw, err := config.NewModelWorker(modelConfig)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, mw)
	s.stub.CheckCallNames(c, "NewWorker", "NewModelWorker")
	s.stub.CheckCall(c, 1, "NewModelWorker", modelConfig)
	config.NewModelWorker = nil

	c.Assert(config, jc.DeepEquals, modelworkermanager.Config{
		Authority:    s.authority,
		ModelWatcher: s.State,
		Mux:          mux,
		Controller:   modelworkermanager.StatePoolController{s.StatePool},
		ErrorDelay:   jworker.RestartDelay,
		Logger:       loggo.GetLogger("test"),
		MachineID:    "1",
	})
}

func (s *ManifoldSuite) TestStopWorkerClosesState(c *gc.C) {
	w := s.startWorkerClean(c)
	defer workertest.CleanKill(c, w)

	s.stateTracker.CheckCallNames(c, "Use")

	workertest.CleanKill(c, w)
	s.stateTracker.CheckCallNames(c, "Use", "Done")
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	workertest.CheckAlive(c, w)
	return w
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

type fakeAgent struct {
	agent.Agent
	agent.Config
}

func (f *fakeAgent) CurrentConfig() agent.Config {
	return f
}

func (f *fakeAgent) Tag() names.Tag {
	return names.NewMachineTag("1")
}
