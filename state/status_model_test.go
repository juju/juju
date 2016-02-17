// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
)

type StatusModelSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&StatusModelSuite{})

func (s *StatusModelSuite) TestUnitAgentStatusDocValidation(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	for i, test := range []struct {
		status status.Status
		info   string
		err    string
	}{{
		status: status.StatusPending,
		err:    `cannot set invalid status "pending"`,
	}, {
		status: status.StatusDown,
		err:    `cannot set invalid status "down"`,
	}, {
		status: status.StatusStarted,
		err:    `cannot set invalid status "started"`,
	}, {
		status: status.StatusStopped,
		err:    `cannot set invalid status "stopped"`,
	}, {
		status: status.StatusAllocating,
		err:    `cannot set status "allocating"`,
	}, {
		status: status.StatusAllocating,
		info:   "any message",
		err:    `cannot set status "allocating"`,
	}, {
		status: status.StatusLost,
		err:    `cannot set status "lost"`,
	}, {
		status: status.StatusLost,
		info:   "any message",
		err:    `cannot set status "lost"`,
	}, {
		status: status.StatusError,
		err:    `cannot set status "error" without info`,
	}, {
		status: status.StatusError,
		info:   "some error info",
	}, {
		status: status.Status("bogus"),
		err:    `cannot set invalid status "bogus"`,
	}} {
		c.Logf("test %d", i)
		err := unit.SetAgentStatus(test.status, test.info, nil)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}
