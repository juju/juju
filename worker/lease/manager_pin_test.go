// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/worker/lease"
)

type PinSuite struct {
	testing.IsolationSuite

	appName    string
	machineTag names.MachineTag
	pinArgs    []interface{}
}

func (s *PinSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.appName = "redis"
	s.machineTag = names.NewMachineTag("0")
	s.pinArgs = []interface{}{
		corelease.Key{
			Namespace: "namespace",
			ModelUUID: "modelUUID",
			Lease:     s.appName,
		},
		s.machineTag,
	}
}

var _ = gc.Suite(&PinSuite{})

func (s *PinSuite) TestPinLease_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "PinLease",
			args:   s.pinArgs,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Pin(s.appName, s.machineTag)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *PinSuite) TestPinLease_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "PinLease",
			args:   s.pinArgs,
			err:    errors.New("boom"),
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Pin(s.appName, s.machineTag)
		c.Check(err, gc.ErrorMatches, "boom")
	})
}

func (s *PinSuite) TestUnpinLease_Success(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "UnpinLease",
			args:   s.pinArgs,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Unpin(s.appName, s.machineTag)
		c.Assert(err, jc.ErrorIsNil)
	})
}

func (s *PinSuite) TestUnpinLease_Error(c *gc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "UnpinLease",
			args:   s.pinArgs,
			err:    errors.New("boom"),
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Unpin(s.appName, s.machineTag)
		c.Check(err, gc.ErrorMatches, "boom")
	})
}

func getPinner(c *gc.C, manager *lease.Manager) corelease.Pinner {
	pinner, err := manager.Pinner("namespace", "modelUUID")
	c.Assert(err, jc.ErrorIsNil)
	return pinner
}
