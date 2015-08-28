// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
)

type statusHistoryFunc func(int) ([]state.StatusInfo, error)

func checkInitialWorkloadStatus(c *gc.C, statusInfo state.StatusInfo) {
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
