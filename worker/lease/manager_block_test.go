// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/lease"
)

type WaitUntilExpiredSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&WaitUntilExpiredSuite{})

func (s *WaitUntilExpiredSuite) TestLeadershipNotHeld(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipExpires(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": corelease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		// Trigger expiry.
		clock.Advance(time.Second)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipChanged(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": corelease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    corelease.ErrInvalid,
			callback: func(leases map[string]corelease.Info) {
				leases["redis"] = corelease.Info{
					Holder: "redis/99",
					Expiry: offset(time.Minute),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		// Trigger abortive expiry.
		clock.Advance(time.Second)
		blockTest.assertBlocked(c)
	})
}

func (s *WaitUntilExpiredSuite) TestLeadershipExpiredEarly(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": corelease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		// Induce a refresh by making an unexpected check; it turns out the
		// lease had already been expired by someone else.
		manager.Token("redis", "redis/99").Check(nil)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *WaitUntilExpiredSuite) TestMultiple(c *gc.C) {
	fix := &Fixture{
		leases: map[string]corelease.Info{
			"redis": corelease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
			"store": corelease.Info{
				Holder: "store/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
		}, {
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    corelease.ErrInvalid,
			callback: func(leases map[string]corelease.Info) {
				delete(leases, "redis")
				leases["store"] = corelease.Info{
					Holder: "store/9",
					Expiry: offset(time.Minute),
				}
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{"store"},
			err:    corelease.ErrInvalid,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *coretesting.Clock) {
		redisTest1 := newBlockTest(manager, "redis")
		redisTest1.assertBlocked(c)
		redisTest2 := newBlockTest(manager, "redis")
		redisTest2.assertBlocked(c)
		storeTest1 := newBlockTest(manager, "store")
		storeTest1.assertBlocked(c)
		storeTest2 := newBlockTest(manager, "store")
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
		leases: map[string]corelease.Info{
			"redis": corelease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		manager.Kill()
		err := blockTest.assertUnblocked(c)
		c.Check(err, gc.ErrorMatches, "lease manager stopped")
	})
}

// blockTest wraps a goroutine running WaitUntilExpired, and fails if it's used
// more than a second after creation (which should be *plenty* of time).
type blockTest struct {
	manager   *lease.Manager
	leaseName string
	done      chan error
	abort     <-chan time.Time
}

// newBlockTest starts a test goroutine blocking until the manager confirms
// expiry of the named lease.
func newBlockTest(manager *lease.Manager, leaseName string) *blockTest {
	bt := &blockTest{
		manager:   manager,
		leaseName: leaseName,
		done:      make(chan error),
		abort:     time.After(time.Second),
	}
	go func() {
		select {
		case <-bt.abort:
		case bt.done <- bt.manager.WaitUntilExpired(bt.leaseName):
		}
	}()
	return bt
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
