// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type ClaimSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ClaimSuite{})

func (s *ClaimSuite) TestClaimLease_Success(c *gc.C) {
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

func (s *ClaimSuite) TestClaimLease_Success_SameHolder(c *gc.C) {
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
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimSuite) TestClaimLease_Failure_OtherHolder(c *gc.C) {
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
					Holder: "redis/1",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, gc.Equals, corelease.ErrClaimDenied)
	})
}

func (s *ClaimSuite) TestClaimLease_Failure_Error(c *gc.C) {
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
		c.Check(err, gc.ErrorMatches, "lease manager stopped")
		err = manager.Wait()
		c.Check(err, gc.ErrorMatches, "lol borken")
	})
}

func (s *ClaimSuite) TestExtendLease_Success(c *gc.C) {
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

func (s *ClaimSuite) TestExtendLease_Success_Expired(c *gc.C) {
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
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimSuite) TestExtendLease_Failure_OtherHolder(c *gc.C) {
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
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getClaimer(c, manager).Claim("redis", "redis/0", time.Minute)
		c.Check(err, gc.Equals, corelease.ErrClaimDenied)
	})
}

func (s *ClaimSuite) TestExtendLease_Failure_Error(c *gc.C) {
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
		c.Check(err, gc.ErrorMatches, "lease manager stopped")
		err = manager.Wait()
		c.Check(err, gc.ErrorMatches, "boom splat")
	})
}

func (s *ClaimSuite) TestOtherHolder_Failure(c *gc.C) {
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
		c.Check(err, gc.Equals, corelease.ErrClaimDenied)
	})
}

func getClaimer(c *gc.C, manager *lease.Manager) corelease.Claimer {
	claimer, err := manager.Claimer("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return claimer
}
