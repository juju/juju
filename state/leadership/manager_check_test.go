// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
)

type CheckLeadershipSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CheckLeadershipSuite{})

func (s *CheckLeadershipSuite) TestSuccess(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": {
				Holder:   "redis/0",
				Expiry:   offset(time.Second),
				AssertOp: txn.Op{C: "fake", Id: "fake"},
			},
		},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("redis", "redis/0")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(token.AssertOps(), jc.DeepEquals, []txn.Op{{
			C: "fake", Id: "fake",
		}})
	})
}

func (s *CheckLeadershipSuite) TestMissingRefresh_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					AssertOp: txn.Op{C: "fake", Id: "fake"},
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("redis", "redis/0")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(token.AssertOps(), jc.DeepEquals, []txn.Op{{
			C: "fake", Id: "fake",
		}})
	})
}

func (s *CheckLeadershipSuite) TestOtherHolderRefresh_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder:   "redis/0",
					Expiry:   offset(time.Second),
					AssertOp: txn.Op{C: "fake", Id: "fake"},
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("redis", "redis/0")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(token.AssertOps(), jc.DeepEquals, []txn.Op{{
			C: "fake", Id: "fake",
		}})
	})
}

func (s *CheckLeadershipSuite) TestRefresh_Failure_Missing(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("redis", "redis/0")
		c.Check(err, gc.ErrorMatches, `"redis/0" is not leader of "redis"`)
		c.Check(token, gc.IsNil)
	})
}

func (s *CheckLeadershipSuite) TestRefresh_Failure_OtherHolder(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			callback: func(leases map[string]lease.Info) {
				leases["redis"] = lease.Info{
					Holder:   "redis/1",
					Expiry:   offset(time.Second),
					AssertOp: txn.Op{C: "fake", Id: "fake"},
				}
			},
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("redis", "redis/0")
		c.Check(err, gc.ErrorMatches, `"redis/0" is not leader of "redis"`)
		c.Check(token, gc.IsNil)
	})
}

func (s *CheckLeadershipSuite) TestRefresh_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			err:    errors.New("crunch squish"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *Clock) {
		token, err := manager.CheckLeadership("redis", "redis/0")
		c.Check(err, gc.ErrorMatches, "leadership manager stopped")
		c.Check(token, gc.IsNil)
		err = manager.Wait()
		c.Check(err, gc.ErrorMatches, "crunch squish")
	})
}
