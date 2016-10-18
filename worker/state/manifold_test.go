// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"errors"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
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
	config          workerstate.ManifoldConfig
	agent           *mockAgent
	resources       dt.StubResources
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.openStateCalled = false
	s.openStateErr = nil

	s.config = workerstate.ManifoldConfig{
		AgentName:              "agent",
		StateConfigWatcherName: "state-config-watcher",
		OpenState:              s.fakeOpenState,
		PingInterval:           10 * time.Millisecond,
	}
	s.manifold = workerstate.Manifold(s.config)
	s.resources = dt.StubResources{
		"agent":                dt.StubResource{Output: new(mockAgent)},
		"state-config-watcher": dt.StubResource{Output: true},
	}
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
		"state-config-watcher",
	})
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	s.resources["agent"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStateConfigWatcherMissing(c *gc.C) {
	s.resources["state-config-watcher"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartOpenStateNil(c *gc.C) {
	s.config.OpenState = nil
	manifold := workerstate.Manifold(s.config)
	w, err := manifold.Start(s.resources.Context())
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "OpenState is nil in config")
}

func (s *ManifoldSuite) TestStartNotStateServer(c *gc.C) {
	s.resources["state-config-watcher"] = dt.StubResource{Output: false}
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
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

	var stTracker workerstate.StateTracker
	err := s.manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)

	st, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(st, gc.Equals, s.State)
	c.Assert(stTracker.Done(), jc.ErrorIsNil)

	// Ensure State is closed when the worker is done.
	checkStop(c, w)
	assertStateClosed(c, s.State)
}

func (s *ManifoldSuite) TestStateStillInUse(c *gc.C) {
	w := s.mustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)

	st, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)

	// Close the worker while the State is still in use.
	checkStop(c, w)
	assertStateNotClosed(c, st)

	// Now signal that the State is no longer needed.
	c.Assert(stTracker.Done(), jc.ErrorIsNil)
	assertStateClosed(c, st)
}

func (s *ManifoldSuite) mustStartManifold(c *gc.C) worker.Worker {
	w, err := s.startManifold(c)
	c.Assert(err, jc.ErrorIsNil)
	return w
}

func (s *ManifoldSuite) startManifold(c *gc.C) (worker.Worker, error) {
	w, err := s.manifold.Start(s.resources.Context())
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
}

func (ma *mockAgent) CurrentConfig() coreagent.Config {
	return new(mockConfig)
}

type mockConfig struct {
	coreagent.Config
}

type dummyWorker struct {
	worker.Worker
}
