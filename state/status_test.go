// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

type statusSuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&statusSuite{})

func (s *statusSuite) TestPruneStatusHistory(c *gc.C) {

	// NOTE: the behaviour is bad, and the test is ugly. I'm just verifying
	// the existing logic here.
	//
	// If you get the opportunity to fix this, you'll want a better shape of
	// test (that injects a usable clock dependency, apart from anything else,
	// and checks that we do our best to maintain a usable span of history
	// rather than an arbitrary limit per entity. And isn't O(N) on status
	// count in the environment).

	const count = 3
	units := make([]*state.Unit, count)
	agents := make([]*state.UnitAgent, count)
	service := s.Factory.MakeService(c, nil)
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{Service: service})
		agents[i] = units[i].Agent().(*state.UnitAgent)
	}

	primeUnitStatusHistory(c, units[0], 10)
	primeUnitStatusHistory(c, units[1], 50)
	primeUnitStatusHistory(c, units[2], 100)
	primeUnitAgentStatusHistory(c, agents[0], 100)
	primeUnitAgentStatusHistory(c, agents[1], 50)
	primeUnitAgentStatusHistory(c, agents[2], 10)

	err := state.PruneStatusHistory(s.State, 30)
	c.Assert(err, jc.ErrorIsNil)

	history, err := units[0].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialUnitStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitStatus(c, statusInfo, 9-i)
	}

	history, err = units[1].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 49-i)
	}

	history, err = units[2].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 99-i)
	}

	history, err = agents[0].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 99-i)
	}

	history, err = agents[1].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 49-i)
	}

	history, err = agents[2].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialUnitAgentStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitAgentStatus(c, statusInfo, 9-i)
	}
}

func (s *statusSuite) TestTranslateLegacyAgentState(c *gc.C) {
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

func (s *statusSuite) TestAgentStatusDocValidation(c *gc.C) {
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
		info:   state.StorageReadyMessage,
	}, {
		status: state.StatusAllocating,
		info:   state.PreparingStorageMessage,
	}, {
		status: state.StatusAllocating,
		info:   "an unexpected or invalid message",
		err:    `cannot set status "allocating"`,
	}, {
		status: state.StatusLost,
		info:   state.StorageReadyMessage,
		err:    `cannot set status "lost"`,
	}, {
		status: state.StatusLost,
		info:   state.PreparingStorageMessage,
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

func checkInitialUnitStatus(c *gc.C, statusInfo state.StatusInfo) {
	c.Check(statusInfo.Status, gc.Equals, state.StatusUnknown)
	c.Check(statusInfo.Message, gc.Equals, "Waiting for agent initialization to finish")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func primeUnitStatusHistory(c *gc.C, unit *state.Unit, count int) {
	for i := 0; i < count; i++ {
		err := unit.SetStatus(state.StatusActive, "", map[string]interface{}{"$foo": i})
		c.Assert(err, gc.IsNil)
	}
}

func checkPrimedUnitStatus(c *gc.C, statusInfo state.StatusInfo, expect int) {
	c.Check(statusInfo.Status, gc.Equals, state.StatusActive)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{"$foo": expect})
	c.Check(statusInfo.Since, gc.NotNil)
}

func checkInitialUnitAgentStatus(c *gc.C, statusInfo state.StatusInfo) {
	c.Check(statusInfo.Status, gc.Equals, state.StatusAllocating)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Assert(statusInfo.Since, gc.NotNil)
}

func primeUnitAgentStatusHistory(c *gc.C, agent *state.UnitAgent, count int) {
	for i := 0; i < count; i++ {
		err := agent.SetStatus(state.StatusExecuting, "", map[string]interface{}{"$bar": i})
		c.Assert(err, gc.IsNil)
	}
}

func checkPrimedUnitAgentStatus(c *gc.C, statusInfo state.StatusInfo, expect int) {
	c.Check(statusInfo.Status, gc.Equals, state.StatusExecuting)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{"$bar": expect})
	c.Check(statusInfo.Since, gc.NotNil)
}
