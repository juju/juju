// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testing/factory"
)

type StatusHistorySuite struct {
	statetesting.StateSuite
}

var _ = gc.Suite(&StatusHistorySuite{})

func (s *StatusHistorySuite) TestPruneStatusHistory(c *gc.C) {

	// NOTE: the behaviour is bad, and the test is ugly. I'm just verifying
	// the existing logic here.
	//
	// If you get the opportunity to fix this, you'll want a better shape of
	// test (that injects a usable clock dependency, apart from anything else,
	// and checks that we do our best to maintain a usable span of history
	// rather than an arbitrary limit per entity. And isn't O(N) on status
	// count in the model).

	const count = 3
	units := make([]*state.Unit, count)
	agents := make([]*state.UnitAgent, count)
	service := s.Factory.MakeService(c, nil)
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{Service: service})
		agents[i] = units[i].Agent()
	}

	primeUnitStatusHistory(c, units[0], 10)
	primeUnitStatusHistory(c, units[1], 50)
	primeUnitStatusHistory(c, units[2], 100)
	primeUnitAgentStatusHistory(c, agents[0], 100)
	primeUnitAgentStatusHistory(c, agents[1], 50)
	primeUnitAgentStatusHistory(c, agents[2], 10)

	err := state.PruneStatusHistory(s.State, 30)
	c.Assert(err, jc.ErrorIsNil)

	history, err := units[0].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialWorkloadStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitStatus(c, statusInfo, 9-i)
	}

	history, err = units[1].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 49-i)
	}

	history, err = units[2].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitStatus(c, statusInfo, 99-i)
	}

	history, err = agents[0].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 99-i)
	}

	history, err = agents[1].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 30)
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 49-i)
	}

	history, err = agents[2].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialUnitAgentStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitAgentStatus(c, statusInfo, 9-i)
	}
}

func (s *StatusHistorySuite) TestStatusHistoryFiltersByDateAndDelta(c *gc.C) {
	service := s.Factory.MakeService(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Service: service})

	twoDaysBack := time.Hour * 48
	threeDaysBack := time.Hour * 72
	err := unit.SetStatus(status.StatusActive, "current status", nil)
	c.Assert(err, jc.ErrorIsNil)
	state.ProbablyUpdateUnitStatusHistory(s.State, unit, status.StatusActive, "2 days ago", twoDaysBack)
	state.ProbablyUpdateUnitStatusHistory(s.State, unit, status.StatusActive, "3 days ago", threeDaysBack)
	history, err := unit.StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 4)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")
	c.Assert(history[2].Message, gc.Equals, "2 days ago")
	c.Assert(history[3].Message, gc.Equals, "3 days ago")
	now := time.Now().Add(10 * time.Second) // lets add some padding to prevent races here.

	// logs up to one day back, using delta.
	oneDayBack := time.Hour * 24
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Delta: &oneDayBack})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")

	// logs up to one day back, using date.
	yesterday := now.Add(-(time.Hour * 24))
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Date: &yesterday})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")

	// Logs up to two days ago, using delta.
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Delta: &twoDaysBack})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")

	// Logs up to two days ago, using date.
	twoDaysAgo := now.Add(-twoDaysBack)
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Date: &twoDaysAgo})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")

	// Logs up to three days ago, using delta.
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Delta: &threeDaysBack})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 3)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")
	c.Assert(history[2].Message, gc.Equals, "2 days ago")

	// Logs up to three days ago, using date.
	threeDaysAgo := now.Add(-threeDaysBack)
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Date: &threeDaysAgo})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 3)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "Waiting for agent initialization to finish")
	c.Assert(history[2].Message, gc.Equals, "2 days ago")
}
