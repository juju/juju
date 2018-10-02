// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type PinSuite struct {
	testing.IsolationSuite
	keyArgs []interface{}
}

func (s *PinSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.keyArgs = []interface{}{
		corelease.Key{
			Namespace: "namespace",
			ModelUUID: "modelUUID",
			Lease:     "redis",
		},
	}
}

var _ = gc.Suite(&PinSuite{})

func (s *PinSuite) TestPinLease_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "PinLease",
			args:   s.keyArgs,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Pin("redis")
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *PinSuite) TestPinLease_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "PinLease",
			args:   s.keyArgs,
			err:    errors.New("boom"),
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Pin("redis")
		c.Check(err, gc.ErrorMatches, "boom")
	})
}

func (s *PinSuite) TestUnpinLease_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "UnpinLease",
			args:   s.keyArgs,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Unpin("redis")
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *PinSuite) TestUnpinLease_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "UnpinLease",
			args:   s.keyArgs,
			err:    errors.New("boom"),
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Unpin("redis")
		c.Check(err, gc.ErrorMatches, "boom")
	})
}

func getPinner(c *gc.C, manager *lease.Manager) corelease.Pinner {
	pinner, err := manager.Pinner("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return pinner
}
