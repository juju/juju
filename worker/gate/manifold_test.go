// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.manifold = gate.Manifold()
	w, err := s.manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	s.worker = w
}

func (s *ManifoldSuite) TearDownTest(c *gc.C) {
	if s.worker != nil {
		checkStop(c, s.worker)
	}
	s.IsolationSuite.TearDownTest(c)
}

func (s *ManifoldSuite) TestLocked(c *gc.C) {
	w := waiter(c, s.manifold, s.worker)
	assertLocked(c, w)
}

func (s *ManifoldSuite) TestUnlock(c *gc.C) {
	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, s.manifold, s.worker)

	u.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestUnlockAgain(c *gc.C) {
	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, s.manifold, s.worker)

	u.Unlock()
	u.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestSameManifoldWorkersConnected(c *gc.C) {
	worker2, err := s.manifold.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	defer checkStop(c, worker2)

	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, s.manifold, worker2)

	u.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestDifferentManifoldWorkersUnconnected(c *gc.C) {
	manifold2 := gate.Manifold()
	worker2, err := manifold2.Start(nil)
	c.Assert(err, jc.ErrorIsNil)
	defer checkStop(c, worker2)

	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, manifold2, worker2)

	u.Unlock()
	assertLocked(c, w)
}

func (s *ManifoldSuite) TestAlreadyUnlockedIsUnlocked(c *gc.C) {
	w := gate.AlreadyUnlocked{}
	assertUnlocked(c, w)
}

func unlocker(c *gc.C, m dependency.Manifold, w worker.Worker) gate.Unlocker {
	var unlocker gate.Unlocker
	err := m.Output(w, &unlocker)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unlocker, gc.NotNil)
	return unlocker
}

func waiter(c *gc.C, m dependency.Manifold, w worker.Worker) gate.Waiter {
	var waiter gate.Waiter
	err := m.Output(w, &waiter)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(waiter, gc.NotNil)
	return waiter
}

func assertLocked(c *gc.C, waiter gate.Waiter) {
	select {
	case <-waiter.Unlocked():
		c.Fatalf("expected gate to be locked")
	default:
	}
}

func assertUnlocked(c *gc.C, waiter gate.Waiter) {
	select {
	case <-waiter.Unlocked():
	default:
		c.Fatalf("expected gate to be unlocked")
	}
}

func checkStop(c *gc.C, w worker.Worker) {
	err := worker.Stop(w)
	c.Check(err, jc.ErrorIsNil)
}
