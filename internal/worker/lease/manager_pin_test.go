// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lease_test

import (
	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/lease"
)

type PinSuite struct {
	testhelpers.IsolationSuite

	appName string
	machine string
	pinArgs []interface{}
}

func (s *PinSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.appName = "redis"
	s.machine = names.NewMachineTag("0").String()
	s.pinArgs = []interface{}{
		corelease.Key{
			Namespace: "namespace",
			ModelUUID: "modelUUID",
			Lease:     s.appName,
		},
		s.machine,
	}
}

var _ = tc.Suite(&PinSuite{})

func (s *PinSuite) TestPinLease_Success(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "PinLease",
			args:   s.pinArgs,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Pin(s.appName, s.machine)
		c.Assert(err, tc.ErrorIsNil)
	})
}

func (s *PinSuite) TestPinLease_Error(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "PinLease",
			args:   s.pinArgs,
			err:    errors.New("boom"),
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Pin(s.appName, s.machine)
		c.Check(err, tc.ErrorMatches, "boom")
	})
}

func (s *PinSuite) TestUnpinLease_Success(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "UnpinLease",
			args:   s.pinArgs,
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Unpin(s.appName, s.machine)
		c.Assert(err, tc.ErrorIsNil)
	})
}

func (s *PinSuite) TestUnpinLease_Error(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "UnpinLease",
			args:   s.pinArgs,
			err:    errors.New("boom"),
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		err := getPinner(c, manager).Unpin(s.appName, s.machine)
		c.Check(err, tc.ErrorMatches, "boom")
	})
}

func (s *PinSuite) TestPinned(c *tc.C) {
	fix := &Fixture{
		expectCalls: []call{{
			method: "Pinned",
		}},
	}
	fix.RunTest(c, func(manager *lease.Manager, _ *testclock.Clock) {
		pinned, err := getPinner(c, manager).Pinned()
		c.Assert(err, tc.ErrorIsNil)
		c.Check(pinned, tc.DeepEquals, map[string][]string{"redis": {s.machine}})
	})
}

func getPinner(c *tc.C, manager *lease.Manager) corelease.Pinner {
	pinner, err := manager.Pinner("namespace", "modelUUID")
	c.Assert(err, tc.ErrorIsNil)
	return pinner
}
