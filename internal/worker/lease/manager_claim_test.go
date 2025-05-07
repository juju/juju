// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"sync"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/mattn/go-sqlite3"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/worker/lease"
)

type ClaimSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ClaimSuite{})

func (s *ClaimSuite) TestClaimLease_Success(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimSuite) TestClaimLease_Success_SameHolder(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
			err: corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/0",
					Expiry: offset(time.Second),
				}
			},
		}, {
			method: "ExtendLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// On the first attempt, we don't see ourselves in the leases, so we try
		// to Claim the lease. But Primary thinks we already have the lease, so it
		// refuses. After claiming, we wait 50ms to let the refresh happen, then
		// we notice that we are the holder, so we Extend instead of Claim.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
			c.Check(err, jc.ErrorIsNil)
			wg.Done()
		}()
		c.Check(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 2), jc.ErrorIsNil)
		wg.Wait()
	})
}

func (s *ClaimSuite) TestClaimLeaseFailureHeldByClaimer(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{Holder: "redis/0", Duration: time.Minute},
			},
			err: corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/1",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// When the Claim starts, it will first get a LeaseInvalid, it will then
		// wait 50ms before trying again, since it is clear that our Leases map
		// does not have the most up-to-date information. We then wake up again
		// and see that our leases have expired and thus let things go.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
			c.Check(err, tc.Equals, corelease.ErrClaimDenied)
			wg.Done()
		}()
		c.Check(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 2), jc.ErrorIsNil)
		wg.Wait()
	})
}

func (s *ClaimSuite) TestClaimLeaseFailureHeldByOther(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{Holder: "redis/0", Duration: time.Minute},
			},
			err: corelease.ErrHeld,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, tc.Equals, corelease.ErrClaimDenied)
	})
}

func (s *ClaimSuite) TestClaimLease_Failure_Error(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
			err: errors.New("lol borken"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, tc.ErrorMatches, "lease manager stopped")
		err = manager.Wait()
		c.Check(err, tc.ErrorMatches, "lol borken")
	})
}

func (s *ClaimSuite) TestExtendLease_Success(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimSuite) TestExtendLease_Success_Expired(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
			err: corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				delete(leases, key("redis"))
			},
		}, {
			method: "ClaimLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// On the first attempt, we think we are the holder, but Primary says "nope".
		// So we wait 50ms for the Leases to get updated. At which point, we have
		// reloaded our Leases and see that *nobody* is the holder. So then we try
		// again and successfully Claim the lease.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
			c.Check(err, jc.ErrorIsNil)
			wg.Done()
		}()
		c.Check(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 2), jc.ErrorIsNil)
		wg.Wait()
	})
}

func (s *ClaimSuite) TestExtendLease_Failure_OtherHolder(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{"redis/0", time.Minute},
			},
			err: corelease.ErrInvalid,
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/1",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// When the Claim starts, it will first get a LeaseInvalid, it will then
		// wait 50ms before trying again, since it is clear that our Leases map
		// does not have the most up-to-date information. We then wake up again
		// and see that our leases have expired and thus let things go.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
			c.Check(err, tc.Equals, corelease.ErrClaimDenied)
			wg.Done()
		}()
		c.Check(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 2), jc.ErrorIsNil)
		wg.Wait()
	})
}

func (s *ClaimSuite) TestExtendLease_Failure_Retryable(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				corelease.Key{
					Namespace: "namespace",
					ModelUUID: "modelUUID",
					Lease:     "redis",
				},
				corelease.Request{Holder: "redis/0", Duration: time.Minute},
			},
			err: sqlite3.ErrLocked,
			callback: func(leases map[corelease.Key]corelease.Info) {
				leases[key("redis")] = corelease.Info{
					Holder: "redis/1",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, clock *testclock.Clock) {
		// When the Claim starts, it will first get a LeaseInvalid, it will then
		// wait 50ms before trying again, since it is clear that our Leases map
		// does not have the most up-to-date information. We then wake up again
		// and see that our leases have expired and thus let things go.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
			c.Check(err, tc.Equals, corelease.ErrClaimDenied)
			wg.Done()
		}()
		c.Check(clock.WaitAdvance(50*time.Millisecond, testing.LongWait, 2), jc.ErrorIsNil)
		wg.Wait()
	})
}

func (s *ClaimSuite) TestExtendLease_Failure_Error(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args: []interface{}{
				key("redis"),
				corelease.Request{"redis/0", time.Minute},
			},
			err: errors.New("boom splat"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, tc.ErrorMatches, "lease manager stopped")
		err = manager.Wait()
		c.Check(err, tc.ErrorMatches, "boom splat")
	})
}

func (s *ClaimSuite) TestOtherHolder_Failure(c *tc.C) {
	fix := &Fixture{
		leases: map[corelease.Key]corelease.Info{
			key("redis"): {
				Holder: "redis/1",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, tc.Equals, corelease.ErrClaimDenied)
	})
}

func getClaimer(c *tc.C, manager *lease.Manager) corelease.Claimer {
	claimer, err := manager.Claimer("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return claimer
}
