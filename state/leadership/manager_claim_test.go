// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreleadership "github.com/juju/juju/leadership"
	"github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
	coretesting "github.com/juju/juju/testing"
)

type ClaimLeadershipSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ClaimLeadershipSuite{})

func (s *ClaimLeadershipSuite) TestClaimLease_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimLeadershipSuite) TestClaimLease_Success_SameHolder(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
			err:    lease.ErrInvalid,
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder: "redis/0",
					Expiry: offset(time.Second),
				}
			},
		}, {
			method: "ExtendLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimLeadershipSuite) TestClaimLease_Failure_OtherHolder(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
			err:    lease.ErrInvalid,
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder: "redis/1",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, gc.Equals, coreleadership.ErrClaimDenied)
	})
}

func (s *ClaimLeadershipSuite) TestClaimLease_Failure_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "ClaimLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
			err:    errors.New("lol borken"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, gc.ErrorMatches, "leadership manager stopped")
		err = manager.Wait()
		c.Check(err, gc.ErrorMatches, "lol borken")
	})
}

func (s *ClaimLeadershipSuite) TestExtendLease_Success(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimLeadershipSuite) TestExtendLease_Success_Expired(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
			err:    lease.ErrInvalid,
			callback: func(leases map[string]lease.Info) {
				delete(leases, "redis")
			},
		}, {
			method: "ClaimLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, jc.ErrorIsNil)
	})
}

func (s *ClaimLeadershipSuite) TestExtendLease_Failure_OtherHolder(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
			err:    lease.ErrInvalid,
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder: "redis/1",
					Expiry: offset(time.Second),
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, gc.Equals, coreleadership.ErrClaimDenied)
	})
}

func (s *ClaimLeadershipSuite) TestExtendLease_Failure_Error(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/0",
				Expiry: offset(time.Second),
			},
		},
		expectCalls: []call{{
			method: "ExtendLease",
			args:   []interface{}{"redis", lease.Request{"redis/0", time.Minute}},
			err:    errors.New("boom splat"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, gc.ErrorMatches, "leadership manager stopped")
		err = manager.Wait()
		c.Check(err, gc.ErrorMatches, "boom splat")
	})
}

func (s *ClaimLeadershipSuite) TestOtherHolder_Failure(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder: "redis/1",
				Expiry: offset(time.Second),
			},
		},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		err := manager.ClaimLeadership("redis", "redis/0", time.Minute)
		c.Check(err, gc.Equals, coreleadership.ErrClaimDenied)
	})
}
