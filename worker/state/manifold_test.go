// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/prometheus/client_golang/prometheus"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/dependency"
	dt "github.com/juju/juju/worker/dependency/testing"
	workerstate "github.com/juju/juju/worker/state"
	"github.com/juju/juju/worker/workertest"
)

type ManifoldSuite struct {
	statetesting.StateSuite
	manifold          dependency.Manifold
	openStateCalled   bool
	openStateErr      error
	config            workerstate.ManifoldConfig
	agent             *mockAgent
	resources         dt.StubResources
	setStatePoolCalls []*state.StatePool
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	// Close the state pool, as it's not needed, and it
	// refers to the state object's mongo session. If we
	// do not close the pool, its embedded watcher may
	// attempt to access mongo after it has been closed
	// by the state tracker.
	err := s.StatePool.Close()
	c.Assert(err, jc.ErrorIsNil)

	s.openStateCalled = false
	s.openStateErr = nil
	s.setStatePoolCalls = nil

	s.config = workerstate.ManifoldConfig{
		AgentName:              "agent",
		StateConfigWatcherName: "state-config-watcher",
		OpenState:              s.fakeOpenState,
		PingInterval:           10 * time.Millisecond,
		PrometheusRegisterer:   prometheus.NewRegistry(),
		SetStatePool: func(pool *state.StatePool) {
			s.setStatePoolCalls = append(s.setStatePoolCalls, pool)
		},
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
	s.startManifoldInvalidConfig(c, s.config, "nil OpenState not valid")
}

func (s *ManifoldSuite) TestStartPrometheusRegistererNil(c *gc.C) {
	s.config.PrometheusRegisterer = nil
	s.startManifoldInvalidConfig(c, s.config, "nil PrometheusRegisterer not valid")
}

func (s *ManifoldSuite) TestStartSetStatePoolNil(c *gc.C) {
	s.config.SetStatePool = nil
	s.startManifoldInvalidConfig(c, s.config, "nil SetStatePool not valid")
}

func (s *ManifoldSuite) startManifoldInvalidConfig(c *gc.C, config workerstate.ManifoldConfig, expect string) {
	manifold := workerstate.Manifold(config)
	w, err := manifold.Start(s.resources.Context())
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
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
	workertest.CleanKill(c, w)
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
	c.Check(err, gc.ErrorMatches, `out should be \*StateTracker; got .+`)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	w := s.mustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)

	pool, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(pool.SystemState(), gc.Equals, s.State)
	c.Assert(stTracker.Done(), jc.ErrorIsNil)

	// Ensure State is closed when the worker is done.
	workertest.CleanKill(c, w)
	assertStateClosed(c, s.State)
}

func (s *ManifoldSuite) TestStateStillInUse(c *gc.C) {
	w := s.mustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)

	pool, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)

	// Close the worker while the State is still in use.
	workertest.CleanKill(c, w)
	assertStateNotClosed(c, pool.SystemState())

	// Now signal that the State is no longer needed.
	c.Assert(stTracker.Done(), jc.ErrorIsNil)
	assertStateClosed(c, pool.SystemState())
}

func (s *ManifoldSuite) TestDeadStateRemoved(c *gc.C) {
	// Create an additional state *before* we start
	// the worker, so the worker's lifecycle watcher
	// is guaranteed to observe it in both the Alive
	// state and the Dead state.
	newSt := s.Factory.MakeModel(c, nil)
	defer newSt.Close()
	model, err := newSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	w := s.mustStartManifold(c)
	defer workertest.CleanKill(c, w)

	var stTracker workerstate.StateTracker
	err = s.manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)
	pool, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	defer stTracker.Done()

	// Get a reference to the state pool entry, so we can
	// prevent it from being fully removed from the pool.
	st, err := pool.Get(newSt.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()

	// Progress the model to Dead.
	err = model.Destroy(state.DestroyModelParams{})
	c.Assert(err, jc.ErrorIsNil)
	err = model.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dead)
	s.State.StartSync()

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		st, err := pool.Get(newSt.ModelUUID())
		if errors.IsNotFound(err) {
			c.Assert(err, gc.ErrorMatches, "model .* has been removed")
			return
		}
		c.Assert(err, jc.ErrorIsNil)
		st.Release()
	}
	c.Fatal("timed out waiting for model state to be removed from pool")
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
