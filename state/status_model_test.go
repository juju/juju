// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
)

type StatusModelSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&StatusModelSuite{})

func (s *StatusModelSuite) TestTranslateLegacyAgentState(c *gc.C) {
	for i, test := range []struct {
		agentStatus     state.Status
		workloadStatus  state.Status
		workloadMessage string
		expected        state.Status
	}{{
		agentStatus: state.StatusAllocating,
		expected:    state.StatusPending,
	}, {
		agentStatus: state.StatusError,
		expected:    state.StatusError,
	}, {
		agentStatus:     state.StatusIdle,
		workloadStatus:  state.StatusMaintenance,
		expected:        state.StatusPending,
		workloadMessage: "installing charm software",
	}, {
		agentStatus:     state.StatusIdle,
		workloadStatus:  state.StatusMaintenance,
		expected:        state.StatusStarted,
		workloadMessage: "backing up",
	}, {
		agentStatus:    state.StatusIdle,
		workloadStatus: state.StatusTerminated,
		expected:       state.StatusStopped,
	}, {
		agentStatus:    state.StatusIdle,
		workloadStatus: state.StatusBlocked,
		expected:       state.StatusStarted,
	}} {
		c.Logf("test %d", i)
		legacy, ok := state.TranslateToLegacyAgentState(test.agentStatus, test.workloadStatus, test.workloadMessage)
		c.Check(ok, jc.IsTrue)
		c.Check(legacy, gc.Equals, test.expected)
	}
}

func (s *StatusModelSuite) TestUnitAgentStatusDocValidation(c *gc.C) {
	unit := s.Factory.MakeUnit(c, nil)
	for i, test := range []struct {
		status state.Status
		info   string
		err    string
	}{{
		status: state.StatusPending,
		err:    `cannot set invalid status "pending"`,
	}, {
		status: state.StatusDown,
		err:    `cannot set invalid status "down"`,
	}, {
		status: state.StatusStarted,
		err:    `cannot set invalid status "started"`,
	}, {
		status: state.StatusStopped,
		err:    `cannot set invalid status "stopped"`,
	}, {
		status: state.StatusAllocating,
		err:    `cannot set status "allocating"`,
	}, {
		status: state.StatusAllocating,
		info:   "any message",
		err:    `cannot set status "allocating"`,
	}, {
		status: state.StatusLost,
		err:    `cannot set status "lost"`,
	}, {
		status: state.StatusLost,
		info:   "any message",
		err:    `cannot set status "lost"`,
	}, {
		status: state.StatusError,
		err:    `cannot set status "error" without info`,
	}, {
		status: state.StatusError,
		info:   "some error info",
	}, {
		status: state.Status("bogus"),
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
