// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
	coretesting "github.com/juju/juju/testing"
)

type BlockUntilLeadershipReleasedSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&BlockUntilLeadershipReleasedSuite{})

func (s *BlockUntilLeadershipReleasedSuite) TestLeadershipNotHeld(c *gc.C) {
	fix := &Fixture{}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *BlockUntilLeadershipReleasedSuite) TestLeadershipExpires(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			callback: func(leases map[string]lease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, clock *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		// Trigger expiry.
		clock.Advance(time.Second)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *BlockUntilLeadershipReleasedSuite) TestLeadershipChanged(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    lease.ErrInvalid,
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder: "redis/99",
					Expiry: offset(time.Minute),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, clock *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		// Trigger abortive expiry.
		clock.Advance(time.Second)
		blockTest.assertBlocked(c)
	})
}

func (s *BlockUntilLeadershipReleasedSuite) TestLeadershipExpiredEarly(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]lease.Info) {
				delete(leases, "redis")
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, clock *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		// Induce a refresh by making an unexpected check; it turns out the
		// lease had already been expired by someone else.
		manager.LeadershipCheck("redis", "redis/99").Check(nil)
		err := blockTest.assertUnblocked(c)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *BlockUntilLeadershipReleasedSuite) TestMultiple(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
			"store": lease.Info{
				Holder: "store/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExpireLease",
			args:   []interface{}{"redis"},
			err:    lease.ErrInvalid,
			callback: func(leases map[string]lease.Info) {
				delete(leases, "redis")
				leases["store"] = lease.Info{
					Holder: "store/9",
					Expiry: offset(time.Minute),
				}
			},
		}, {
			method: "ExpireLease",
			args:   []interface{}{"store"},
			err:    lease.ErrInvalid,
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, clock *coretesting.Clock) {
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

func (s *BlockUntilLeadershipReleasedSuite) TestKillManager(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		blockTest := newBlockTest(manager, "redis")
		blockTest.assertBlocked(c)

		manager.Kill()
		err := blockTest.assertUnblocked(c)
		c.Check(err, gc.ErrorMatches, "leadership manager stopped")
	})
}

// blockTest wraps a goroutine running BlockUntilLeadershipReleased, and
// fails if it's used more than a second after creation (which should be
// *plenty* of time).
type blockTest struct {
	manager     leadership.ManagerWorker
	serviceName string
	done        chan error
	abort       <-chan time.Time
}

// newBlockTest starts a test goroutine blocking until the manager confirms
// leaderlessness of the named service.
func newBlockTest(manager leadership.ManagerWorker, serviceName string) *blockTest {
	bt := &blockTest{
		manager:     manager,
		serviceName: serviceName,
		done:        make(chan error),
		abort:       time.After(time.Second),
	}
	go func() {
		select {
		case <-bt.abort:
		case bt.done <- bt.manager.BlockUntilLeadershipReleased(bt.serviceName):
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
