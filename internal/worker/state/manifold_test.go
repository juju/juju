// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	mgotesting "github.com/juju/mgo/v3/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/internal/services"
	coretesting "github.com/juju/juju/internal/testing"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type ManifoldSuite struct {
	statetesting.StateSuite
	manifold          dependency.Manifold
	openStateCalled   bool
	openStateErr      error
	config            workerstate.ManifoldConfig
	resources         dt.StubResources
	setStatePoolCalls []*state.StatePool
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.openStateCalled = false
	s.openStateErr = nil
	s.setStatePoolCalls = nil

	s.config = workerstate.ManifoldConfig{
		AgentName:              "agent",
		StateConfigWatcherName: "state-config-watcher",
		DomainServicesName:     "domain-services",
		OpenStatePool:          s.fakeOpenState,
		PingInterval:           10 * time.Millisecond,
		SetStatePool: func(pool *state.StatePool) {
			s.setStatePoolCalls = append(s.setStatePoolCalls, pool)
		},
	}
	s.manifold = workerstate.Manifold(s.config)
	s.resources = dt.StubResources{
		"agent":                dt.NewStubResource(new(mockAgent)),
		"state-config-watcher": dt.NewStubResource(true),
		"domain-services":      dt.NewStubResource(new(mockDomainServices)),
	}
}

func (s *ManifoldSuite) fakeOpenState(context.Context, coreagent.Config, services.DomainServicesGetter) (*state.StatePool, error) {
	s.openStateCalled = true
	if s.openStateErr != nil {
		return nil, s.openStateErr
	}
	// Here's one we prepared earlier...
	return s.StatePool, nil
}

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.manifold.Inputs, jc.SameContents, []string{
		"agent",
		"state-config-watcher",
		"domain-services",
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

func (s *ManifoldSuite) TestDomainServicesMissing(c *gc.C) {
	s.resources["domain-services"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartOpenStateNil(c *gc.C) {
	s.config.OpenStatePool = nil
	s.startManifoldInvalidConfig(c, s.config, "nil OpenStatePool not valid")
}

func (s *ManifoldSuite) TestStartSetStatePoolNil(c *gc.C) {
	s.config.SetStatePool = nil
	s.startManifoldInvalidConfig(c, s.config, "nil SetStatePool not valid")
}

func (s *ManifoldSuite) startManifoldInvalidConfig(c *gc.C, config workerstate.ManifoldConfig, expect string) {
	manifold := workerstate.Manifold(config)
	w, err := manifold.Start(context.Background(), s.resources.Getter())
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *ManifoldSuite) TestStartNotStateServer(c *gc.C) {
	s.resources["state-config-watcher"] = dt.NewStubResource(false)
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(err, gc.ErrorMatches, "no StateServingInfo in config: dependency not available")
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
	mgotesting.MgoServer.Destroy()

	// FIXME: Ideally we'd want the "state ping failed" error here, but in reality the txn watcher will fail
	// first because it is long polling.
	checkExitsWithError(c, w, "(state ping failed|hub txn watcher sync error): .+")
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
	pool, _, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	defer stTracker.Done()

	// Get a reference to the state pool entry, so we can
	// prevent it from being fully removed from the pool.
	st, err := pool.Get(newSt.ModelUUID())
	c.Assert(err, jc.ErrorIsNil)
	defer st.Release()

	// Progress the model to Dead.
	c.Assert(model.Destroy(state.DestroyModelParams{}), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIsNil)
	c.Assert(model.Life(), gc.Equals, state.Dying)
	c.Assert(newSt.RemoveDyingModel(), jc.ErrorIsNil)
	c.Assert(model.Refresh(), jc.ErrorIs, errors.NotFound)

	for a := coretesting.LongAttempt.Start(); a.Next(); {
		st, err := pool.Get(newSt.ModelUUID())
		if errors.Is(err, errors.NotFound) {
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
	w, err := s.manifold.Start(context.Background(), s.resources.Getter())
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

type mockDomainServices struct {
	services.ControllerDomainServices
	services.DomainServicesGetter
}
