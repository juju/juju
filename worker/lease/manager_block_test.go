// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type WaitUntilExpiredSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WaitUntilExpiredSuite{})

func (s *WaitUntilExpiredSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	logger := loggo.GetLogger("juju.worker.lease")
	logger.SetLogLevel(loggo.TRACE)
	logger = loggo.GetLogger("lease_test")
	logger.SetLogLevel(loggo.TRACE)
}

func (s *WaitUntilExpiredSuite) TestLeadershipNoLeaseBlockEvaluatedNextTick(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "postgresql/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)

		// Check that *another* lease expiry causes the unassociated block to
		// be checked and in the absence of its lease, get unblocked.
		c.Assert(clock.WaitAdvance(2*time.Second, testing.ShortWait, 1), jc.ErrorIsNil)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipExpires(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)

		// Trigger expiry.
		c.Assert(clock.WaitAdvance(2*time.Second, testing.ShortWait, 1), jc.ErrorIsNil)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestBlockChecksRescheduled(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("postgresql"): {
				Holder: "postgresql/0",
				Expiry: offset(time.Second),
			},
			key("mysql"): {
				Holder: "mysql/0",
				Expiry: offset(4 * time.Second),
			},
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(7 * time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)

		// Advance past the first expiry.
		c.Assert(clock.WaitAdvance(3*time.Second, testing.ShortWait, 1), jc.ErrorIsNil)
		blockTest.assertBlocked(c)

		// Advance past the second expiry. We should have had a check scheduled.
		c.Assert(clock.WaitAdvance(3*time.Second, testing.ShortWait, 1), jc.ErrorIsNil)
		blockTest.assertBlocked(c)

		// Advance past the last expiry. We should have had a check scheduled
		// that causes the redis lease to be unblocked.
		c.Assert(clock.WaitAdvance(3*time.Second, testing.ShortWait, 1), jc.ErrorIsNil)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipChanged(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)

		// Trigger abortive expiry.
		clock.Advance(time.Second)
		blockTest.assertBlocked(c)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipExpiredEarly(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			// The lease is held by an entity other than the checker.
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)

		// Induce a scheduled block check by making an unexpected check;
		// it turns out the lease had already been expired by someone else.
		checker, err := manager.Checker("namespace", "model")
		c.Assert(err, jc.ErrorIsNil)
		err = checker.Token("redis", "redis/99").Check(0, nil)
		c.Assert(err, gc.ErrorMatches, "lease not held")

		// Simulate the delayed synchronisation by removing the lease.
		delete(fix.leases, key("redis"))

		// When we notice that we are out of sync, we should queue up an
		// expiration and update of blockers after a very short timeout.
		err = clock.WaitAdvance(time.Second, testing.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)

		err = blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestMultiple(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(2 * time.Second),
			},
			key("store"): {
				Holder: "store/0",
				Expiry: offset(2 * time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		redisTest1 := newBlockTest(c, manager, key("redis"))
		redisTest1.assertBlocked(c)
		redisTest2 := newBlockTest(c, manager, key("redis"))
		redisTest2.assertBlocked(c)
		storeTest1 := newBlockTest(c, manager, key("store"))
		storeTest1.assertBlocked(c)
		storeTest2 := newBlockTest(c, manager, key("store"))
		storeTest2.assertBlocked(c)

		// Induce a scheduled block check by making an unexpected check.
		checker, err := manager.Checker("namespace", "model")
		c.Assert(err, jc.ErrorIsNil)
		err = checker.Token("redis", "redis/99").Check(0, nil)
		c.Assert(err, gc.ErrorMatches, "lease not held")

		// Deleting the redis lease should cause unblocks for the redis
		// blockers, but the store blocks should remain.
		delete(fix.leases, key("redis"))

		err = clock.WaitAdvance(time.Second, testing.ShortWait, 1)
		c.Assert(err, jc.ErrorIsNil)

		err = redisTest2.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)

		err = redisTest1.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)

		storeTest2.assertBlocked(c)
		storeTest1.assertBlocked(c)
	})
}

func (s *WaitUntilExpiredSuite) TestKillManager(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)

		manager.Kill()
		err := blockTest.assertUnblocked(c)
		c.Check(err, gc.ErrorMatches, "lease manager stopped")
	})
}

func (s *WaitUntilExpiredSuite) TestCancelWait(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		blockTest := newBlockTest(c, manager, key("redis"))
		blockTest.assertBlocked(c)
		blockTest.cancelWait()

		err := blockTest.assertUnblocked(c)
		c.Check(err, gc.Equals, corelease.ErrWaitCancelled)
		c.Check(err, gc.ErrorMatches, "waiting for lease cancelled by client")
	})
}

// blockTest wraps a goroutine running WaitUntilExpired, and fails if it's used
// more than a second after creation (which should be *plenty* of time).
type blockTest struct {
	manager *lease.Manager
	done    chan error
	abort   <-chan time.Time
	cancel  chan struct{}
}

// newBlockTest starts a test goroutine blocking until the manager confirms
// expiry of the named lease.
func newBlockTest(c *gc.C, manager *lease.Manager, key corelease.Key) *blockTest {
	bt := &blockTest{
		manager: manager,
		done:    make(chan error),
		abort:   time.After(time.Second),
		cancel:  make(chan struct{}),
	}
	claimer, err := bt.manager.Claimer(key.Namespace, key.ModelUUID)
	if err != nil {
		c.Errorf("couldn't get claimer: %v", err)
	}
	started := make(chan struct{})
	go func() {
		close(started)
		select {
		case <-bt.abort:
		case bt.done <- claimer.WaitUntilExpired(key.Lease, bt.cancel):
		case <-time.After(testing.LongWait):
			c.Errorf("block not aborted or expired after %v", testing.LongWait)
		}
	}()
	select {
	case <-started:
	case <-bt.abort:
		c.Errorf("bt.aborted before even started")
	}
	return bt
}

func (bt *blockTest) cancelWait() {
	close(bt.cancel)
}

func (bt *blockTest) assertBlocked(c *gc.C) {
	select {
	case err := <-bt.done:
		c.Errorf("unblocked unexpectedly with %v", err)
	case <-time.After(testing.ShortWait):
		// Happy that we are still blocked; success.
	}
}

func (bt *blockTest) assertUnblocked(c *gc.C) error {
	lease.ManagerStore(bt.manager).(*Store).expireLeases()
	select {
	case err := <-bt.done:
		return err
	case <-bt.abort:
		c.Errorf("timed out before unblocking")
		return errors.Errorf("timed out")
	}
}
