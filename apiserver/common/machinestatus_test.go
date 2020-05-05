// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"errors"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type MachineStatusSuite struct {
	testing.IsolationSuite
	ctx     common.ModelPresenceContext
	machine *mockMachine
}

var _ = gc.Suite(&MachineStatusSuite{})

func (s *MachineStatusSuite) SetUpTest(c *gc.C) {
	s.machine = &mockMachine{
		id:     "666",
		status: status.Started,
	}
	s.ctx = common.ModelPresenceContext{
		Presence: agentAlive(names.NewMachineTag(s.machine.id).String()),
	}
}

func (s *MachineStatusSuite) checkUntouched(c *gc.C) {
	agent, err := s.ctx.MachineStatus(s.machine)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(agent.Status, jc.DeepEquals, s.machine.status)
}

func (s *MachineStatusSuite) TestNormal(c *gc.C) {
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestErrors(c *gc.C) {
	s.machine.statusErr = errors.New("status error")

	_, err := s.ctx.MachineStatus(s.machine)
	c.Assert(err, gc.ErrorMatches, "status error")
}

func (s *MachineStatusSuite) TestDown(c *gc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	agent, err := s.ctx.MachineStatus(s.machine)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(agent, jc.DeepEquals, status.StatusInfo{
		Status:  status.Down,
		Message: "agent is not communicating with the server",
	})
}

func (s *MachineStatusSuite) TestDownAndDead(c *gc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	s.machine.life = state.Dead
	// Status is untouched if unit is Dead.
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestPresenceError(c *gc.C) {
	s.ctx.Presence = presenceError(names.NewMachineTag(s.machine.Id()).String())
	// Presence error gets ignored, so no output is unchanged.
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestNotDownIfPending(c *gc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	s.machine.status = status.Pending
	s.checkUntouched(c)
}
