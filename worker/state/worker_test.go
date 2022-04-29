// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/worker/v3"

	jujutesting "github.com/juju/testing"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

type WorkerSuite struct {
	BaseSuite
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestStartSuccess(c *gc.C) {
	w := s.MustStartManifold(c)
	c.Check(s.OpenStateCalled, jc.IsTrue)
	checkNotExiting(c, w)
	workertest.CleanKill(c, w)
}

func (s *WorkerSuite) TestStatePinging(c *gc.C) {
	// Exhaust the retries ASAP.
	s.Config.PingInterval = time.Millisecond
	s.Manifold = Manifold(s.Config)

	w := s.MustStartManifold(c)
	checkNotExiting(c, w)

	jujutesting.MgoServer.Destroy()

	err := workertest.CheckKilled(c, w)
	c.Check(err, gc.ErrorMatches, "state ping failed: .+")
}

func (s *WorkerSuite) TestDeadStateRemoved(c *gc.C) {
	// Create an additional state *before* we start
	// the worker, so the worker's lifecycle watcher
	// is guaranteed to observe it in both the Alive
	// state and the Dead state.
	newSt := s.Factory.MakeModel(c, nil)
	defer func() { _ = newSt.Close() }()
	model, err := newSt.Model()
	c.Assert(err, jc.ErrorIsNil)

	w := s.MustStartManifold(c)
	defer workertest.CleanKill(c, w)

	var stTracker StateTracker
	err = s.Manifold.Output(w, &stTracker)
	c.Assert(err, jc.ErrorIsNil)
	pool, err := stTracker.Use()
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = stTracker.Done() }()

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
	c.Assert(model.Refresh(), jc.Satisfies, errors.IsNotFound)
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

func checkNotExiting(c *gc.C, w worker.Worker) {
	exited := make(chan bool)
	go func() {
		_ = w.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		c.Fatal("worker exited unexpectedly")
	case <-time.After(coretesting.ShortWait):
		// Worker didn't exit (good)
	}
}
