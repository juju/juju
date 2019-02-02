// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock/testclock"
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

func (s *WaitUntilExpiredSuite) TestLeadershipNotHeld(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		blockTest := newBlockTest(manager, key("redis"))
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
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(manager, key("redis"))
		blockTest.assertBlocked(c)

		// Trigger expiry.
		clock.Advance(time.Second)
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
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			err:    corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/99",
					Expiry: offset(time.Minute),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(manager, key("redis"))
		blockTest.assertBlocked(c)

		// Trigger abortive expiry.
		clock.Advance(time.Second)
		blockTest.assertBlocked(c)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipExpiredEarly(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		blockTest := newBlockTest(manager, key("redis"))
		blockTest.assertBlocked(c)

		// Induce a refresh by making an unexpected check; it turns out the
		// lease had already been expired by someone else.
		checker, err := manager.Checker("namespace", "model")
		c.Assert(err, jc.ErrorIsNil)
		checker.Token("redis", "redis/99").Check(0, nil)
		err = blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestMultiple(c *gc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
			key("store"): {
				Holder: "store/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("redis")},
			err:    corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
				leases[key("store")] = corelease.Info{
					Holder: "store/9",
					Expiry: offset(time.Minute),
				}
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{key("store")},
			err:    corelease.ErrInvalid,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		redisTest1 := newBlockTest(manager, key("redis"))
		redisTest1.assertBlocked(c)
		redisTest2 := newBlockTest(manager, key("redis"))
		redisTest2.assertBlocked(c)
		storeTest1 := newBlockTest(manager, key("store"))
		storeTest1.assertBlocked(c)
		storeTest2 := newBlockTest(manager, key("store"))
		storeTest2.assertBlocked(c)

		// Induce attempted expiry; redis was expired already, store was
		// refreshed and not expired.
		clock.Advance(time.Second)
		err := redisTest2.assertUnblocked(c)
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
		blockTest := newBlockTest(manager, key("redis"))
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
		blockTest := newBlockTest(manager, key("redis"))
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
func newBlockTest(manager *lease.Manager, key corelease.Key) *blockTest {
	bt := &blockTest{
		manager: manager,
		done:    make(chan error),
		abort:   time.After(time.Second),
		cancel:  make(chan struct{}),
	}
	claimer, err := bt.manager.Claimer(key.Namespace, key.ModelUUID)
	if err != nil {
		panic("couldn't get claimer")
	}
	go func() {
		select {
		case <-bt.abort:
		case bt.done <- claimer.WaitUntilExpired(key.Lease, bt.cancel):
		}
	}()
	return bt
}

func (bt *blockTest) cancelWait() {
	close(bt.cancel)
}

func (bt *blockTest) assertBlocked(c *gc.C) {
	select {
	case err := <-bt.done:
		c.Fatalf("unblocked unexpectedly with %v", err)
	default:
	}
}

func (bt *blockTest) assertUnblocked(c *gc.C) error {
	select {
	case err := <-bt.done:
		return err
	case <-bt.abort:
		c.Fatalf("timed out before unblocking")
	}
	panic("unreachable")
}
