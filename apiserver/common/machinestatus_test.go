// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

type MachineStatusSuite struct {
	testing.IsolationSuite
	ctx     common.ModelPresenceContext
	machine *fakeMachine
}

var _ = gc.Suite(&MachineStatusSuite{})

func (s *MachineStatusSuite) SetUpTest(c *gc.C) {
	s.machine = &fakeMachine{
		id:     "666",
		status: status.Started,
	}
	s.ctx = common.ModelPresenceContext{
		Presence: agentAlive(names.NewMachineTag(s.machine.id).String()),
	}
}

func (s *MachineStatusSuite) checkUntouched(c *gc.C) {
	agent, err := s.ctx.MachineStatus(context.Background(), s.machine)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(agent.Status, jc.DeepEquals, s.machine.status)
}

func (s *MachineStatusSuite) TestNormal(c *gc.C) {
	s.checkUntouched(c)
}

func (s *MachineStatusSuite) TestErrors(c *gc.C) {
	s.machine.statusErr = errors.New("status error")

	_, err := s.ctx.MachineStatus(context.Background(), s.machine)
	c.Assert(err, gc.ErrorMatches, "status error")
}

func (s *MachineStatusSuite) TestDown(c *gc.C) {
	s.ctx.Presence = agentDown(names.NewMachineTag(s.machine.Id()).String())
	agent, err := s.ctx.MachineStatus(context.Background(), s.machine)
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

type fakeMachine struct {
	state.Machine
	id                 string
	life               state.Life
	hw                 *instance.HardwareCharacteristics
	instId             instance.Id
	displayName        string
	status             status.Status
	statusErr          error
	destroyErr         error
	forceDestroyErr    error
	forceDestroyCalled bool
	destroyCalled      bool
}

func (m *fakeMachine) Id() string {
	return m.id
}

func (m *fakeMachine) Life() state.Life {
	return m.life
}

func (m *fakeMachine) InstanceId() (instance.Id, error) {
	return m.instId, nil
}

func (m *fakeMachine) InstanceNames() (instance.Id, string, error) {
	instId, err := m.InstanceId()
	return instId, m.displayName, err
}

func (m *fakeMachine) Status() (status.StatusInfo, error) {
	return status.StatusInfo{
		Status: m.status,
	}, m.statusErr
}

func (m *fakeMachine) HardwareCharacteristics() (*instance.HardwareCharacteristics, error) {
	return m.hw, nil
}

func (m *fakeMachine) ForceDestroy(time.Duration) error {
	m.forceDestroyCalled = true
	if m.forceDestroyErr != nil {
		return m.forceDestroyErr
	}
	m.life = state.Dying
	return nil
}

func (m *fakeMachine) Destroy(_ objectstore.ObjectStore) error {
	m.destroyCalled = true
	if m.destroyErr != nil {
		return m.destroyErr
	}
	m.life = state.Dying
	return nil
}
