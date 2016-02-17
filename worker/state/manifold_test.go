// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"errors"
	"time"

	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	workerstate "github.com/juju/juju/worker/state"
)

type ManifoldSuite struct {
	statetesting.StateSuite
	manifold        dependency.Manifold
	openStateCalled bool
	openStateErr    error
	agent           *mockAgent
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.openStateCalled = false
	s.openStateErr = nil
	s.agent = new(mockAgent)
	s.agent.conf.SetStateServingInfo(params.StateServingInfo{})
	s.agent.conf.tag = names.NewMachineTag("99")

	s.manifold = workerstate.Manifold(workerstate.ManifoldConfig{
		AgentName:              "agent",
		AgentConfigUpdatedName: "agent-config-updated",
		OpenState:              s.fakeOpenState,
		PingInterval:           10 * time.Millisecond,
	})
}

func (s *ManifoldSuite) fakeOpenState(coreagent.Config) (*state.State, error) {
	s.openStateCalled = true
	if s.openStateErr != nil {
		return nil, s.openStateErr
	}
	// Here's one we prepared earlier...
	return s.State, nil
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"agent",
		"agent-config-updated",
	})
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent": dt.StubResource{Error: dependency.ErrMissing},
	})
	worker, err := s.manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartOpenStateNil(c *gc.C) {
	manifold := workerstate.Manifold(workerstate.ManifoldConfig{
		AgentName:              "agent",
		AgentConfigUpdatedName: "agent-config-updated",
	})
	getResource := dt.StubGetResource(dt.StubResources{
		"agent": dt.StubResource{Output: s.agent},
	})
	worker, err := manifold.Start(getResource)
	c.Check(worker, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "OpenState is nil in config")
}

func (s *ManifoldSuite) TestStartNotStateServer(c *gc.C) {
	s.agent.conf.ssiSet = false
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNotAMachineAgent(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("foo/0")
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "manifold may only be used in a machine agent")
}

func (s *ManifoldSuite) TestStartOpenStateFails(c *gc.C) {
	s.openStateErr = errors.New("boom")
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	w := s.mustStartManifold(c)
	c.Check(s.openStateCalled, jc.IsTrue)
	checkNotExiting(c, w)
	checkStop(c, w)
}

func (s *ManifoldSuite) TestStatePinging(c *gc.C) {
	w := s.mustStartManifold(c)
	checkNotExiting(c, w)

	// Kill the mongod to cause pings to fail.
	jujutesting.MgoServer.Destroy()

	checkExitsWithError(c, w, "state ping failed: .+")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var st *state.State
	err := s.manifold.Output(dummyWorker{}, &st)
	c.Check(st, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `in should be a \*state.stateWorker; .+`)
}

func (s *ManifoldSuite) TestOutputWrongType(c *gc.C) {
	w := s.mustStartManifold(c)

	var wrong int
	err := s.manifold.Output(w, &wrong)
	c.Check(wrong, gc.Equals, 0)
	c.Check(err, gc.ErrorMatches, `out should be \*state.State; got .+`)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	w := s.mustStartManifold(c)

	var st *state.State
	err := s.manifold.Output(w, &st)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(st, gc.Equals, s.State)
}

func (s *ManifoldSuite) mustStartManifold(c *gc.C) worker.Worker {
	w, err := s.startManifold(c)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *ManifoldSuite) startManifold(c *gc.C) (worker.Worker, error) {
	getResource := dt.StubGetResource(dt.StubResources{
		"agent": dt.StubResource{Output: s.agent},
	})
	w, err := s.manifold.Start(getResource)
	if w != nil {
		s.AddCleanup(func(*gc.C) { worker.Stop(w) })
	}
	return w, err
}

func checkStop(c *gc.C, w worker.Worker) {
	err := worker.Stop(w)
	c.Check(err, jc.ErrorIsNil)
}

func checkNotExiting(c *gc.C, w worker.Worker) {
	exited := make(chan bool)
	go func() {
		w.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		c.Fatal("worker exited unexpectedly")
	case <-time.After(coretesting.ShortWait):
		// Worker didn't exit (good)
	}
}

func checkExitsWithError(c *gc.C, w worker.Worker, expectedErr string) {
	errCh := make(chan error)
	go func() {
		errCh <- w.Wait()
	}()
	select {
	case err := <-errCh:
		c.Check(err, gc.ErrorMatches, expectedErr)
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to exit")
	}
}

type mockAgent struct {
	coreagent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() coreagent.Config {
	return &ma.conf
}

func (ma *mockAgent) ChangeConfig(f coreagent.ConfigMutator) error {
	return f(&ma.conf)
}

type mockConfig struct {
	coreagent.ConfigSetter
	tag    names.Tag
	ssiSet bool
	ssi    params.StateServingInfo
}

func (mc *mockConfig) Tag() names.Tag {
	return mc.tag
}

func (mc *mockConfig) StateServingInfo() (params.StateServingInfo, bool) {
	if mc.ssiSet {
		return mc.ssi, true
	} else {
		return params.StateServingInfo{}, false
	}
}

func (mc *mockConfig) SetStateServingInfo(info params.StateServingInfo) {
	mc.ssiSet = true
	mc.ssi = info
}

type dummyWorker struct {
	worker.Worker
}
