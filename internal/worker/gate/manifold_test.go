// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gate_test

import (
	"context"

	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/juju/worker/v4/workertest"

	"github.com/juju/juju/internal/worker/gate"
)

type ManifoldSuite struct {
	testing.IsolationSuite
	manifold dependency.Manifold
	worker   worker.Worker
}

var _ = tc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.manifold = gate.Manifold()
	w, err := s.manifold.Start(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	s.worker = w
}

func (s *ManifoldSuite) TearDownTest(c *tc.C) {
	if s.worker != nil {
		checkStop(c, s.worker)
	}
	s.IsolationSuite.TearDownTest(c)
}

func (s *ManifoldSuite) TestLocked(c *tc.C) {
	w := waiter(c, s.manifold, s.worker)
	assertLocked(c, w)
}

func (s *ManifoldSuite) TestUnlock(c *tc.C) {
	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, s.manifold, s.worker)

	u.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestUnlockAgain(c *tc.C) {
	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, s.manifold, s.worker)

	u.Unlock()
	u.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestRestartLocks(c *tc.C) {
	u := unlocker(c, s.manifold, s.worker)
	u.Unlock()

	workertest.CleanKill(c, s.worker)
	worker, err := s.manifold.Start(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	w := waiter(c, s.manifold, worker)
	assertLocked(c, w)
}

func (s *ManifoldSuite) TestManifoldWithLockWorkersConnected(c *tc.C) {
	lock := gate.NewLock()
	manifold := gate.ManifoldEx(lock)
	worker, err := manifold.Start(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker)

	worker2, err := manifold.Start(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, worker2)

	u := unlocker(c, manifold, worker)
	w := waiter(c, manifold, worker2)

	u.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestLockOutput(c *tc.C) {
	var lock gate.Lock
	err := s.manifold.Output(s.worker, &lock)
	c.Assert(err, tc.ErrorIsNil)

	w := waiter(c, s.manifold, s.worker)
	assertLocked(c, w)
	lock.Unlock()
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestDifferentManifoldWorkersUnconnected(c *tc.C) {
	manifold2 := gate.Manifold()
	worker2, err := manifold2.Start(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer checkStop(c, worker2)

	u := unlocker(c, s.manifold, s.worker)
	w := waiter(c, manifold2, worker2)

	u.Unlock()
	assertLocked(c, w)
}

func (s *ManifoldSuite) TestAlreadyUnlockedIsUnlocked(c *tc.C) {
	w := gate.AlreadyUnlocked{}
	assertUnlocked(c, w)
}

func (s *ManifoldSuite) TestManifoldEx(c *tc.C) {
	lock := gate.NewLock()

	manifold := gate.ManifoldEx(lock)
	var waiter1 gate.Waiter = lock
	var unlocker1 gate.Unlocker = lock

	worker, err := manifold.Start(context.Background(), nil)
	c.Assert(err, tc.ErrorIsNil)
	defer checkStop(c, worker)
	waiter2 := waiter(c, manifold, worker)

	assertLocked(c, waiter1)
	assertLocked(c, waiter2)

	unlocker1.Unlock()
	assertUnlocked(c, waiter1)
	assertUnlocked(c, waiter2)
}

func unlocker(c *tc.C, m dependency.Manifold, w worker.Worker) gate.Unlocker {
	var unlocker gate.Unlocker
	err := m.Output(w, &unlocker)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unlocker, tc.NotNil)
	return unlocker
}

func waiter(c *tc.C, m dependency.Manifold, w worker.Worker) gate.Waiter {
	var waiter gate.Waiter
	err := m.Output(w, &waiter)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(waiter, tc.NotNil)
	return waiter
}

func assertLocked(c *tc.C, waiter gate.Waiter) {
	c.Assert(waiter.IsUnlocked(), tc.IsFalse)
	select {
	case <-waiter.Unlocked():
		c.Fatalf("expected gate to be locked")
	default:
	}
}

func assertUnlocked(c *tc.C, waiter gate.Waiter) {
	c.Assert(waiter.IsUnlocked(), tc.IsTrue)
	select {
	case <-waiter.Unlocked():
	default:
		c.Fatalf("expected gate to be unlocked")
	}
}

func checkStop(c *tc.C, w worker.Worker) {
	err := worker.Stop(w)
	c.Check(err, tc.ErrorIsNil)
}
