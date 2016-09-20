// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type MachineStatusSuite struct {
	testing.IsolationSuite
	machine *mockMachine
}

var _ = gc.Suite(&MachineStatusSuite{})

func (s *MachineStatusSuite) SetUpTest(c *gc.C) {
	s.machine = &mockMachine{
		status: status.Started,
	}
}

func (s *MachineStatusSuite) checkUntouched(c *gc.C) {
	agent, err := common.MachineStatus(s.machine)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(agent.Status, jc.DeepEquals, s.machine.status)
}

func (s *MachineStatusSuite) TestNormal(c *gc.C) {
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestErrors(c *gc.C) {
	s.machine.statusErr = errors.New("status error")

	_, err := common.MachineStatus(s.machine)
	c.Assert(err, gc.ErrorMatches, "status error")
}

func (s *MachineStatusSuite) TestDown(c *gc.C) {
	s.machine.agentDead = true
	agent, err := common.MachineStatus(s.machine)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agent, jc.DeepEquals, status.StatusInfo{
		Status:  status.Down,
		Message: "agent is not communicating with the server",
	})
}

func (s *MachineStatusSuite) TestDownAndDead(c *gc.C) {
	s.machine.agentDead = true
	s.machine.life = state.Dead
	// Status is untouched if unit is Dead.
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestPresenceError(c *gc.C) {
	s.machine.agentDead = true
	s.machine.presenceErr = errors.New("boom")
	// Presence error gets ignored, so no output is unchanged.
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestNotDownIfPending(c *gc.C) {
	s.machine.agentDead = true
	s.machine.status = status.Pending
	s.checkUntouched(c)
}
