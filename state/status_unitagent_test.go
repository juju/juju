// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"time" // Only used for time types.

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type StatusUnitAgentSuite struct {
	ConnSuite
	unit  *state.Unit
	agent *state.UnitAgent
}

var _ = gc.Suite(&StatusUnitAgentSuite{})

func (s *StatusUnitAgentSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.unit = s.Factory.MakeUnit(c, nil)
	s.agent = s.unit.Agent()
}

func (s *StatusUnitAgentSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.agent.Status()
	c.Check(err, jc.ErrorIsNil)
	checkInitialUnitAgentStatus(c, statusInfo)
}

func (s *StatusUnitAgentSuite) TestSetUnknownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetErrorStatusWithoutInfo(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetAllocatingStatusAlreadyAssigned(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Allocating,
		Message: "",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "allocating" as unit is already assigned`)

	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetStatusUnassigned(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "foo"})
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	agent := u.Agent()
	for _, value := range []status.Status{status.Idle, status.Executing, status.Rebooting, status.Failed} {
		now := testing.ZeroTime()
		sInfo := status.StatusInfo{
			Status:  value,
			Message: "",
			Since:   &now,
		}
		err := agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
		c.Check(err, gc.ErrorMatches, fmt.Sprintf(`cannot set status %q until unit is assigned`, value))

		s.checkInitialStatus(c)
	}
}

func (s *StatusUnitAgentSuite) TestSetStatusRunningNonCAAS(c *gc.C) {
	app := s.Factory.MakeApplication(c, &factory.ApplicationParams{Name: "foo"})
	u, err := app.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	agent := u.Agent()
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Running,
		Message: "",
		Since:   &now,
	}
	err = agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set invalid status "running"`)
	s.checkInitialStatus(c)
}

func (s *StatusUnitAgentSuite) TestSetOverwritesData(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "something",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) checkGetSetStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "something",
		Data: map[string]interface{}{
			"$foo":    "bar",
			"baz.qux": "ping",
			"pong": map[string]interface{}{
				"$unset": "txn-revno",
			},
		},
		Since: &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)

	unit, err := s.State.Unit(s.unit.Name())
	c.Assert(err, jc.ErrorIsNil)
	agent := unit.Agent()

	statusInfo, err := agent.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Idle)
	c.Check(statusInfo.Message, gc.Equals, "something")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$foo":    "bar",
		"baz.qux": "ping",
		"pong": map[string]interface{}{
			"$unset": "txn-revno",
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusDying(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusDead(c *gc.C) {
	preventUnitDestroyRemove(c, s.unit)
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)
	err = s.unit.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *StatusUnitAgentSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.unit.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "not really",
		Since:   &now,
	}
	err = s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status: agent not found`)

	statusInfo, err := s.agent.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: agent not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *StatusUnitAgentSuite) TestGetSetErrorStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "test-hook failed",
		Data: map[string]interface{}{
			"foo": "bar",
		},
		Since: &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)

	// Agent error is reported as unit error.
	statusInfo, err := s.unit.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Error)
	c.Check(statusInfo.Message, gc.Equals, "test-hook failed")
	c.Check(statusInfo.Data, gc.DeepEquals, map[string]interface{}{
		"foo": "bar",
	})

	// For agents, error is reported as idle.
	statusInfo, err = s.agent.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Idle)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
}

func timeBeforeOrEqual(timeBefore, timeOther time.Time) bool {
	return timeBefore.Before(timeOther) || timeBefore.Equal(timeOther)
}

func (s *StatusUnitAgentSuite) TestSetAgentStatusSince(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err := s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := s.agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	firstTime := statusInfo.Since
	c.Assert(firstTime, gc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), jc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	now = now.Add(1 * time.Second)
	sInfo = status.StatusInfo{
		Status:  status.Idle,
		Message: "",
		Since:   &now,
	}
	err = s.agent.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = s.agent.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), jc.IsTrue)
}

func (s *StatusUnitAgentSuite) TestStatusHistoryInitial(c *gc.C) {
	history, err := s.agent.StatusHistory(status.StatusHistoryFilter{Size: 1})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 1)

	checkInitialUnitAgentStatus(c, history[0])
}

func (s *StatusUnitAgentSuite) TestStatusHistoryShort(c *gc.C) {
	primeUnitAgentStatusHistory(c, s.Clock, s.agent, 5, 0, "")

	history, err := s.agent.StatusHistory(status.StatusHistoryFilter{Size: 10})
	c.Check(err, jc.ErrorIsNil)
	c.Assert(history, gc.HasLen, 6)

	checkInitialUnitAgentStatus(c, history[5])
	history = history[:5]
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 4-i, 0)
	}
}

func (s *StatusUnitAgentSuite) TestStatusHistoryLong(c *gc.C) {
	primeUnitAgentStatusHistory(c, s.Clock, s.agent, 25, 0, "")

	history, err := s.agent.StatusHistory(status.StatusHistoryFilter{Size: 15})
	c.Check(err, jc.ErrorIsNil)
	c.Check(history, gc.HasLen, 15)
	for i, statusInfo := range history {
		checkPrimedUnitAgentStatus(c, statusInfo, 24-i, 0)
	}
}
