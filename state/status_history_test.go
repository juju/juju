// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	"github.com/juju/testing"
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

func (s *StatusHistorySuite) TestPruneStatusHistoryBySize(c *gc.C) {
	clock := testing.NewClock(time.Now())
	err := s.State.SetClockForTesting(clock)
	c.Assert(err, jc.ErrorIsNil)
	service := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: service})
	state.PrimeUnitStatusHistory(c, clock, unit, status.Active, 20000, 1000, nil)

	history, err := unit.StatusHistory(status.StatusHistoryFilter{Size: 25000})
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("%d\n", len(history))
	c.Assert(history, gc.HasLen, 20001)

	err = state.PruneStatusHistory(s.State, 0, 1)
	c.Assert(err, jc.ErrorIsNil)

	history, err = unit.StatusHistory(status.StatusHistoryFilter{Size: 25000})
	c.Assert(err, jc.ErrorIsNil)
	historyLen := len(history)
	// When writing this test, the size was 6670 for about 0,00015 MB per entry
	// but that is a size that can most likely change so I wont risk a flaky test
	// here, enough to say that if this size suddenly is no longer less than
	// half its good reason for suspicion.
	c.Assert(historyLen, jc.LessThan, 10000)
}

func (s *StatusHistorySuite) TestPruneStatusHistoryByDate(c *gc.C) {

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
	service := s.Factory.MakeApplication(c, nil)
	for i := 0; i < count; i++ {
		units[i] = s.Factory.MakeUnit(c, &factory.UnitParams{Application: service})
		agents[i] = units[i].Agent()
	}

	primeUnitStatusHistory(c, units[0], 10, 0)
	primeUnitStatusHistory(c, units[0], 10, 24*time.Hour)
	primeUnitStatusHistory(c, units[1], 50, 0)
	primeUnitStatusHistory(c, units[1], 50, 24*time.Hour)
	primeUnitStatusHistory(c, units[2], 100, 0)
	primeUnitStatusHistory(c, units[2], 100, 24*time.Hour)
	primeUnitAgentStatusHistory(c, agents[0], 100, 0)
	primeUnitAgentStatusHistory(c, agents[0], 100, 24*time.Hour)
	primeUnitAgentStatusHistory(c, agents[1], 50, 0)
	primeUnitAgentStatusHistory(c, agents[1], 50, 24*time.Hour)
	primeUnitAgentStatusHistory(c, agents[2], 10, 0)
	primeUnitAgentStatusHistory(c, agents[2], 10, 24*time.Hour)

	history, err := units[0].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 21)
	checkInitialWorkloadStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitStatus(c, statusInfo, 9-i, 0)
	}
	for i, statusInfo := range history[11:20] {
		checkPrimedUnitStatus(c, statusInfo, 9-i, 24*time.Hour)
	}

	err = state.PruneStatusHistory(s.State, 10*time.Hour, 1024)
	c.Assert(err, jc.ErrorIsNil)

	history, err = units[0].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialWorkloadStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitStatus(c, statusInfo, 9-i, 0)
	}

	history, err = units[1].StatusHistory(status.StatusHistoryFilter{Size: 100})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 51)
	for i, statusInfo := range history[:50] {
		checkPrimedUnitStatus(c, statusInfo, 49-i, 0)
	}

	history, err = units[2].StatusHistory(status.StatusHistoryFilter{Size: 200})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 101)
	for i, statusInfo := range history[:100] {
		checkPrimedUnitStatus(c, statusInfo, 99-i, 0)
	}

	history, err = agents[0].StatusHistory(status.StatusHistoryFilter{Size: 200})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 101)
	for i, statusInfo := range history[:100] {
		checkPrimedUnitAgentStatus(c, statusInfo, 99-i, 0)
	}

	history, err = agents[1].StatusHistory(status.StatusHistoryFilter{Size: 100})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 51)
	for i, statusInfo := range history[:50] {
		checkPrimedUnitAgentStatus(c, statusInfo, 49-i, 0)
	}

	history, err = agents[2].StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialUnitAgentStatus(c, history[10])
	for i, statusInfo := range history[:10] {
		checkPrimedUnitAgentStatus(c, statusInfo, 9-i, 0)
	}
}

func (s *StatusHistorySuite) TestStatusHistoryFiltersByDateAndDelta(c *gc.C) {
	// TODO(perrito666) setup should be extracted into a fixture and the
	// 6 or 7 test cases each get their own method.
	service := s.Factory.MakeApplication(c, nil)
	unit := s.Factory.MakeUnit(c, &factory.UnitParams{Application: service})

	twoDaysBack := time.Hour * 48
	threeDaysBack := time.Hour * 72
	now := time.Now()
	twoDaysAgo := now.Add(-twoDaysBack)
	threeDaysAgo := now.Add(-threeDaysBack)
	sInfo := status.StatusInfo{
		Status:  status.Active,
		Message: "current status",
		Since:   &now,
	}
	err := unit.SetStatus(sInfo)
	c.Assert(err, jc.ErrorIsNil)
	sInfo = status.StatusInfo{
		Status:  status.Active,
		Message: "2 days ago",
		Since:   &twoDaysAgo,
	}
	unit.SetStatus(sInfo)
	sInfo = status.StatusInfo{
		Status:  status.Active,
		Message: "3 days ago",
		Since:   &threeDaysAgo,
	}
	unit.SetStatus(sInfo)
	history, err := unit.StatusHistory(status.StatusHistoryFilter{Size: 50})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 4)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")
	c.Assert(history[2].Message, gc.Equals, "2 days ago")
	c.Assert(history[3].Message, gc.Equals, "3 days ago")
	now = now.Add(10 * time.Second) // lets add some padding to prevent races here.

	// logs up to one day back, using delta.
	oneDayBack := time.Hour * 24
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Delta: &oneDayBack})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")

	// logs up to one day back, using date.
	yesterday := now.Add(-(time.Hour * 24))
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Date: &yesterday})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")

	// Logs up to two days ago, using delta.
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Delta: &twoDaysBack})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")

	// Logs up to two days ago, using date.

	history, err = unit.StatusHistory(status.StatusHistoryFilter{Date: &twoDaysAgo})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 2)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")

	// Logs up to three days ago, using delta.
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Delta: &threeDaysBack})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 3)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")
	c.Assert(history[2].Message, gc.Equals, "2 days ago")

	// Logs up to three days ago, using date.
	history, err = unit.StatusHistory(status.StatusHistoryFilter{Date: &threeDaysAgo})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 3)
	c.Assert(history[0].Message, gc.Equals, "current status")
	c.Assert(history[1].Message, gc.Equals, "waiting for machine")
	c.Assert(history[2].Message, gc.Equals, "2 days ago")
}
