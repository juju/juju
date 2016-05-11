// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"runtime"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type statusHistoryFunc func(int) ([]status.StatusInfo, error)

type statusSetter interface {
	SetStatus(status.Status, string, map[string]interface{}) error
}

func primeStatusHistory(c *gc.C, entity statusSetter, statusVal status.Status, count int, nextData func(int) map[string]interface{}) {
	info := ""
	for i := 0; i < count; i++ {
		c.Logf("setting status for %v", entity)
		data := nextData(i)
		err := entity.SetStatus(statusVal, info, data)
		c.Assert(err, jc.ErrorIsNil)
		if runtime.GOOS == "windows" {
			// The default clock tick on Windows is 15.6 ms.
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func checkInitialWorkloadStatus(c *gc.C, statusInfo status.StatusInfo) {
	c.Check(statusInfo.Status, gc.Equals, status.StatusUnknown)
	c.Check(statusInfo.Message, gc.Equals, "Waiting for agent initialization to finish")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func primeUnitStatusHistory(c *gc.C, unit *state.Unit, count int) {
	primeStatusHistory(c, unit, status.StatusActive, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"$foo": i}
	})
}

func checkPrimedUnitStatus(c *gc.C, statusInfo status.StatusInfo, expect int) {
	c.Check(statusInfo.Status, gc.Equals, status.StatusActive)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{"$foo": expect})
	c.Check(statusInfo.Since, gc.NotNil)
}

func checkInitialUnitAgentStatus(c *gc.C, statusInfo status.StatusInfo) {
	c.Check(statusInfo.Status, gc.Equals, status.StatusAllocating)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Assert(statusInfo.Since, gc.NotNil)
}

func primeUnitAgentStatusHistory(c *gc.C, agent *state.UnitAgent, count int) {
	primeStatusHistory(c, agent, status.StatusExecuting, count, func(i int) map[string]interface{} {
		return map[string]interface{}{"$bar": i}
	})
}

func checkPrimedUnitAgentStatus(c *gc.C, statusInfo status.StatusInfo, expect int) {
	c.Check(statusInfo.Status, gc.Equals, status.StatusExecuting)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{"$bar": expect})
	c.Check(statusInfo.Since, gc.NotNil)
}
