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
	// count in the environment).

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

	history, err := units[0].StatusHistory(50)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 11)
	checkInitialWorkloadStatus(c, history[10])
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
