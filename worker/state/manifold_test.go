// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"
	dt "github.com/juju/worker/v3/dependency/testing"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	workerstate "github.com/juju/juju/worker/state"
)

type ManifoldSuite struct {
	workerstate.BaseSuite
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	c.Assert(s.Manifold.Inputs, jc.SameContents, []string{
		"agent",
		"state-config-watcher",
	})
}

func (s *ManifoldSuite) TestStartAgentMissing(c *gc.C) {
	s.Resources["agent"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.StartManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStateConfigWatcherMissing(c *gc.C) {
	s.Resources["state-config-watcher"] = dt.StubResource{Error: dependency.ErrMissing}
	w, err := s.StartManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.Equals, dependency.ErrMissing)
}

func (s *ManifoldSuite) TestStartOpenStateNil(c *gc.C) {
	s.Config.OpenStatePool = nil
	s.startManifoldInvalidConfig(c, s.Config, "nil OpenStatePool not valid")
}

func (s *ManifoldSuite) TestStartSetStatePoolNil(c *gc.C) {
	s.Config.SetStatePool = nil
	s.startManifoldInvalidConfig(c, s.Config, "nil SetStatePool not valid")
}

func (s *ManifoldSuite) startManifoldInvalidConfig(c *gc.C, config workerstate.ManifoldConfig, expect string) {
	manifold := workerstate.Manifold(config)
	w, err := manifold.Start(s.Resources.Context())
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}

func (s *ManifoldSuite) TestStartNotStateServer(c *gc.C) {
	s.Resources["state-config-watcher"] = dt.NewStubResource(false)
	w, err := s.StartManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	c.Check(err, gc.ErrorMatches, "no StateServingInfo in config: dependency not available")
}

func (s *ManifoldSuite) TestStartOpenStateFails(c *gc.C) {
	s.OpenStateErr = errors.New("boom")
	w, err := s.StartManifold(c)
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "boom")
}

func (s *ManifoldSuite) TestOutputBadWorker(c *gc.C) {
	var st *state.State
	err := s.Manifold.Output(dummyWorker{}, &st)
	c.Check(st, gc.IsNil)
	c.Check(err, gc.ErrorMatches, `in should be a \*state.stateWorker; .+`)
}

func (s *ManifoldSuite) TestOutputWrongType(c *gc.C) {
	w := s.MustStartManifold(c)
	defer workertest.DirtyKill(c, w)

	var wrong int
	err := s.Manifold.Output(w, &wrong)
	c.Check(wrong, gc.Equals, 0)
	c.Check(err, gc.ErrorMatches, `out should be \*StateTracker; got .+`)
}

func (s *ManifoldSuite) TestOutputSuccess(c *gc.C) {
	w := s.MustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.Manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)

	pool, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(pool.SystemState(), gc.Equals, s.State)
	c.Assert(stTracker.Done(), jc.ErrorIsNil)

	// Ensure State is closed when the worker is done.
	workertest.CleanKill(c, w)
	assertStatePoolClosed(c, s.StatePool)
}

func (s *ManifoldSuite) TestStateStillInUse(c *gc.C) {
	w := s.MustStartManifold(c)

	var stTracker workerstate.StateTracker
	err := s.Manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)

	pool, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)

	// Close the worker while the State is still in use.
	workertest.CleanKill(c, w)
	assertStatePoolNotClosed(c, pool)

	// Now signal that the State is no longer needed.
	c.Assert(stTracker.Done(), jc.ErrorIsNil)
	assertStatePoolClosed(c, pool)
}

type dummyWorker struct {
	worker.Worker
}
