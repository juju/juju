// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateconfigwatcher_test

import (
	"sync"
	"time"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/voyeur"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/stateconfigwatcher"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	agent              *mockAgent
	goodContext        dependency.Context
	agentConfigChanged *voyeur.Value
	manifold           dependency.Manifold
	worker             worker.Worker
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.agent = new(mockAgent)
	s.agent.conf.setStateServingInfo(true)
	s.agent.conf.tag = names.NewMachineTag("99")

	s.goodContext = dt.StubContext(nil, map[string]interface{}{
		"agent": s.agent,
	})

	s.agentConfigChanged = voyeur.NewValue(0)
	s.manifold = stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
		AgentName:          "agent",
		AgentConfigChanged: s.agentConfigChanged,
	})
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{"agent"})
}

func (s *ManifoldSuite) TestNoAgent(c *gc.C) {
	context := dt.StubContext(nil, map[string]interface{}{
		"agent": dependency.ErrMissing,
	})
	_, err := s.manifold.Start(context)
	c.Assert(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestNilAgentConfigChanged(c *gc.C) {
	manifold := stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
		AgentName: "agent",
	})
	_, err := manifold.Start(s.goodContext)
	c.Assert(err, gc.ErrorMatches, "nil AgentConfigChanged .+")
}

func (s *ManifoldSuite) TestNotMachineAgent(c *gc.C) {
	s.agent.conf.tag = names.NewUnitTag("foo/0")
	_, err := s.manifold.Start(s.goodContext)
	c.Assert(err, gc.ErrorMatches, "manifold can only be used with a machine or controller agent")
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	w, err := s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	checkStop(c, w)
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var out bool
	err := s.manifold.Output(dummyWorker{}, &out)
	c.Check(err, gc.ErrorMatches, `in should be a \*stateconfigwatcher.stateConfigWatcher; .+`)
}

func (s *ManifoldSuite) TestOutputWrongType(c *gc.C) {
	w, err := s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	defer checkStop(c, w)

	var out int
	err = s.manifold.Output(w, &out)
	c.Check(err, gc.ErrorMatches, `out should be \*bool; got .+`)
}

func (s *ManifoldSuite) TestOutputSuccessNotStateServer(c *gc.C) {
	s.agent.conf.setStateServingInfo(false)
	w, err := s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	defer checkStop(c, w)

	var out bool
	err = s.manifold.Output(w, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out, jc.IsFalse)
}

func (s *ManifoldSuite) TestOutputSuccessStateServer(c *gc.C) {
	s.agent.conf.setStateServingInfo(true)
	w, err := s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	defer checkStop(c, w)

	var out bool
	err = s.manifold.Output(w, &out)
	c.Check(err, jc.ErrorIsNil)
	c.Check(out, jc.IsTrue)
}

func (s *ManifoldSuite) TestBounceOnChange(c *gc.C) {
	s.agent.conf.setStateServingInfo(false)
	w, err := s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	checkNotExiting(c, w)

	checkOutput := func(expected bool) {
		var out bool
		err = s.manifold.Output(w, &out)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(out, gc.Equals, expected)
	}

	// Not a state server yet, initial output should be False.
	checkOutput(false)

	// Changing the config without changing the state server status -
	// worker should keep running and output should remain false.
	s.agentConfigChanged.Set(0)
	checkNotExiting(c, w)
	checkOutput(false)

	// Now change the config to include state serving info, worker
	// should bounce.
	s.agent.conf.setStateServingInfo(true)
	s.agentConfigChanged.Set(0)
	checkExitsWithError(c, w, dependency.ErrBounce)

	// Restart the worker, the output should now be true.
	w, err = s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	checkNotExiting(c, w)
	checkOutput(true)

	// Changing the config again without changing the state serving
	// info shouldn't cause the agent to exit.
	s.agentConfigChanged.Set(0)
	checkNotExiting(c, w)
	checkOutput(true)

	// Now remove the state serving info, the agent should bounce.
	s.agent.conf.setStateServingInfo(false)
	s.agentConfigChanged.Set(0)
	checkExitsWithError(c, w, dependency.ErrBounce)
}

func (s *ManifoldSuite) TestClosedVoyeur(c *gc.C) {
	w, err := s.manifold.Start(s.goodContext)
	c.Assert(err, jc.ErrorIsNil)
	checkNotExiting(c, w)

	s.agentConfigChanged.Close()

	c.Check(waitForExit(c, w), gc.ErrorMatches, "config changed value closed")
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

func checkExitsWithError(c *gc.C, w worker.Worker, expectedErr error) {
	c.Check(waitForExit(c, w), gc.Equals, expectedErr)
}

func waitForExit(c *gc.C, w worker.Worker) error {
	errCh := make(chan error)
	go func() {
		errCh <- w.Wait()
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(coretesting.LongWait):
		c.Fatal("timed out waiting for worker to exit")
	}
	panic("can't get here")
}

type mockAgent struct {
	coreagent.Agent
	conf mockConfig
}

func (ma *mockAgent) CurrentConfig() coreagent.Config {
	return &ma.conf
}

type mockConfig struct {
	coreagent.ConfigSetter
	tag         names.Tag
	mu          sync.Mutex
	ssInfoIsSet bool
}

func (mc *mockConfig) Tag() names.Tag {
	return mc.tag
}

func (mc *mockConfig) setStateServingInfo(isSet bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.ssInfoIsSet = isSet
}

func (mc *mockConfig) StateServingInfo() (controller.StateServingInfo, bool) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	return controller.StateServingInfo{}, mc.ssInfoIsSet
}

type dummyWorker struct {
	worker.Worker
}
