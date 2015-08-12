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

	coreleadership "github.com/juju/juju/leadership"
	"github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
	coretesting "github.com/juju/juju/testing"
)

type LeadershipCheckSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LeadershipCheckSuite{})

func (s *LeadershipCheckSuite) TestSuccess(c *gc.C) {
	fix := &Fixture{
		leases: map[string]lease.Info{
			"redis": lease.Info{
				Holder:   "redis/0",
				Expiry:   offset(time.Second),
				AssertOp: txn.Op{C: "fake", Id: "fake"},
			},
		},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(assertOps(c, token), jc.DeepEquals, []txn.Op{{
			C: "fake", Id: "fake",
		}})
	})
}

func (s *LeadershipCheckSuite) TestMissingRefresh_Success(c *gc.C) {
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
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(assertOps(c, token), jc.DeepEquals, []txn.Op{{
			C: "fake", Id: "fake",
		}})
	})
}

func (s *LeadershipCheckSuite) TestOtherHolderRefresh_Success(c *gc.C) {
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
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(assertOps(c, token), jc.DeepEquals, []txn.Op{{
			C: "fake", Id: "fake",
		}})
	})
}

func (s *LeadershipCheckSuite) TestRefresh_Failure_Missing(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
		}},
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(token.Check(nil), gc.ErrorMatches, `"redis/0" is not leader of "redis"`)
	})
}

func (s *LeadershipCheckSuite) TestRefresh_Failure_OtherHolder(c *gc.C) {
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
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(token.Check(nil), gc.ErrorMatches, `"redis/0" is not leader of "redis"`)
	})
}

func (s *LeadershipCheckSuite) TestRefresh_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Refresh",
			err:    errors.New("crunch squish"),
		}},
		expectDirty: true,
	}
	fix.RunTest(c, func(manager leadership.ManagerWorker, _ *coretesting.Clock) {
		token := manager.LeadershipCheck("redis", "redis/0")
		c.Check(token.Check(nil), gc.ErrorMatches, "leadership manager stopped")
		err := manager.Wait()
		c.Check(err, gc.ErrorMatches, "crunch squish")
	})
}

func assertOps(c *gc.C, token coreleadership.Token) (out []txn.Op) {
	err := token.Check(&out)
	c.Check(err, jc.ErrorIsNil)
	return out
}
