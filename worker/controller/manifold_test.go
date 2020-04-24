// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller_test

import (
	"errors"
	"time"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	coretesting "github.com/juju/juju/testing"
	workercontroller "github.com/juju/juju/worker/controller"
)

type ManifoldSuite struct {
	statetesting.StateSuite
	manifold             dependency.Manifold
	openControllerCalled bool
	openControllerErr    error
	config               workercontroller.ManifoldConfig
	agent                *mockAgent
	resources            dt.StubResources
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)

	s.openControllerCalled = false
	s.openControllerErr = nil

	s.config = workercontroller.ManifoldConfig{
		AgentName:              "agent",
		StateConfigWatcherName: "state-config-watcher",
		OpenController:         s.fakeOpenController,
		MongoPingInterval:      10 * time.Millisecond,
	}
	s.manifold = workercontroller.Manifold(s.config)
	s.resources = dt.StubResources{
		"agent":                dt.NewStubResource(new(mockAgent)),
		"state-config-watcher": dt.NewStubResource(true),
	}
}

func (s *ManifoldSuite) fakeOpenController(coreagent.Config) (*state.Controller, error) {
	s.openControllerCalled = true
	if s.openControllerErr != nil {
		return nil, s.openControllerErr
	}
	// Here's one we prepared earlier...
	return s.Controller, nil
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

func (s *ManifoldSuite) TestStartOpenControllerNil(c *gc.C) {
	s.config.OpenController = nil
	manifold := workercontroller.Manifold(s.config)
	w, err := manifold.Start(s.resources.Context())
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "OpenController is nil in config")
}

func (s *ManifoldSuite) TestStartNotController(c *gc.C) {
	s.resources["state-config-watcher"] = dt.NewStubResource(false)
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartOpenControllerFails(c *gc.C) {
	s.openControllerErr = errors.New("boom")
	w, err := s.startManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestStartSuccess(c *gc.C) {
	w := s.mustStartManifold(c)
	c.Check(s.openControllerCalled, jc.IsTrue)
	checkNotExiting(c, w)
	checkStop(c, w)
}

func (s *ManifoldSuite) TestPinging(c *gc.C) {
	w := s.mustStartManifold(c)
	checkNotExiting(c, w)

	// Kill the mongod to cause pings to fail.
	jujutesting.MgoServer.Destroy()

	checkExitsWithError(c, w, "database ping failed: .+")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var st *state.Controller
	err := s.manifold.Output(dummyWorker{}, &st)
	c.Check(st, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `in should be a \*controller.controllerWorker; .+`)
}

func (s *ManifoldSuite) TestOutputWrongType(c *gc.C) {
	w := s.mustStartManifold(c)

	var wrong int
	err := s.manifold.Output(w, &wrong)
	c.Check(wrong, gc.Equals, 0)
	c.Check(err, gc.ErrorMatches, `out should be \*Tracker; got .+`)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	w := s.mustStartManifold(c)

	var tracker workercontroller.Tracker
	err := s.manifold.Output(w, &tracker)
	c.Assert(err, jc.ErrorIsNil)

	ctlr, err := tracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ctlr, gc.Equals, s.Controller)
	c.Assert(tracker.Done(), jc.ErrorIsNil)

	// Ensure Controller is closed when the worker is done.
	checkStop(c, w)
	assertControllerClosed(c, s.Controller)
}

func (s *ManifoldSuite) TestControllerStillInUse(c *gc.C) {
	w := s.mustStartManifold(c)

	var tracker workercontroller.Tracker
	err := s.manifold.Output(w, &tracker)
	c.Assert(err, jc.ErrorIsNil)

	ctlr, err := tracker.Use()
	c.Assert(err, jc.ErrorIsNil)

	// Close the worker while the State is still in use.
	checkStop(c, w)
	assertControllerNotClosed(c, ctlr)

	// Now signal that the Controller is no longer needed.
	c.Assert(tracker.Done(), jc.ErrorIsNil)
	assertControllerClosed(c, ctlr)
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
