// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
)

type MachineStatusSuite struct {
	ConnSuite
	machine *state.Machine
}

var _ = gc.Suite(&MachineStatusSuite{})

func (s *MachineStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.machine = s.Factory.MakeMachine(c, nil)
}

func (s *MachineStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.machine.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Pending)
	c.Check(statusInfo.Message, gc.Equals, "")
	c.Check(statusInfo.Data, gc.HasLen, 0)
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *MachineStatusSuite) TestSetErrorStatusWithoutInfo(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Error,
		Message: "",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "error" without info`)

	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) TestSetDownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Down,
		Message: "",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "down"`)

	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) TestSetUnknownStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Status("vliegkat"),
		Message: "orville",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Assert(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *MachineStatusSuite) TestSetOverwritesData(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Data: map[string]interface{}{
			"pew.pew": "zap",
		},
		Since: &now,
	}
	err := s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) checkGetSetStatus(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "blah",
		Data: map[string]interface{}{
			"$foo.bar.baz": map[string]interface{}{
				"pew.pew": "zap",
			},
		},
		Since: &now,
	}
	err := s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)

	machine, err := s.State.Machine(s.machine.Id())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := machine.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.Started)
	c.Check(statusInfo.Message, gc.Equals, "blah")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$foo.bar.baz": map[string]interface{}{
			"pew.pew": "zap",
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *MachineStatusSuite) TestGetSetStatusDying(c *gc.C) {
	err := s.machine.Destroy(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) TestGetSetStatusDead(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)

	// NOTE: it would be more technically correct to reject status updates
	// while Dead, but it's easier and clearer, not to mention more efficient,
	// to just depend on status doc existence.
	s.checkGetSetStatus(c)
}

func (s *MachineStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.machine.EnsureDead()
	c.Assert(err, jc.ErrorIsNil)
	err = s.machine.Remove(state.NewObjectStore(c, s.State.ModelUUID()))
	c.Assert(err, jc.ErrorIsNil)

	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Started,
		Message: "not really",
		Since:   &now,
	}
	err = s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status: machine not found`)

	statusInfo, err := s.machine.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: machine not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *MachineStatusSuite) TestSetStatusPendingProvisioned(c *gc.C) {
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err := s.machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, gc.ErrorMatches, `cannot set status "pending"`)
}

func (s *MachineStatusSuite) TestSetStatusPendingUnprovisioned(c *gc.C) {
	machine, err := s.State.AddMachine(defaultInstancePrechecker, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	now := testing.ZeroTime()
	sInfo := status.StatusInfo{
		Status:  status.Pending,
		Message: "",
		Since:   &now,
	}
	err = machine.SetStatus(sInfo, status.NoopStatusHistoryRecorder)
	c.Check(err, jc.ErrorIsNil)
}
