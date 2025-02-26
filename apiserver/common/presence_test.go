// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/presence"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/state"
)

func agentAlive(agent string) common.ModelPresence {
	return &fakeModelPresence{status: presence.Alive, agent: agent}
}

func agentDown(agent string) common.ModelPresence {
	return &fakeModelPresence{status: presence.Missing, agent: agent}
}

func presenceError(agent string) common.ModelPresence {
	return &fakeModelPresence{err: errors.New("boom"), agent: agent}
}

type fakeModelPresence struct {
	agent  string
	status presence.Status
	err    error
}

func (f *fakeModelPresence) AgentStatus(agent string) (presence.Status, error) {
	if agent != f.agent {
		return f.status, fmt.Errorf("unexpected agent %v, expected %v", agent, f.agent)
	}
	return f.status, f.err
}

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

type UnitStatusSuite struct {
	testing.IsolationSuite
	ctx  common.ModelPresenceContext
	unit *fakeStatusUnit
}

var _ = gc.Suite(&UnitStatusSuite{})

func (s *UnitStatusSuite) SetUpTest(c *gc.C) {
	c.Skip("skipping factory based tests. TODO: Re-write without factories")
	s.unit = &fakeStatusUnit{
		app:      "foo",
		presence: true,
	}
	s.ctx = common.ModelPresenceContext{
		Presence: agentAlive(names.NewUnitTag(s.unit.Name()).String()),
	}
}

var (
	agentStatus = status.StatusInfo{
		Status:  status.Started,
		Message: "agent ok",
	}
	workloadStatus = status.StatusInfo{
		Status:  status.Idle,
		Message: "unit ok",
	}
)

func (s *UnitStatusSuite) checkUntouched(c *gc.C, agentStatus, workloadStatus status.StatusInfo) {
	agent, workload := s.ctx.UnitStatus(context.Background(), s.unit, agentStatus, workloadStatus)
	c.Check(agent, jc.DeepEquals, agentStatus)
	c.Check(workload, jc.DeepEquals, workloadStatus)
}

func (s *UnitStatusSuite) checkLost(c *gc.C, agentStatus, workloadStatus status.StatusInfo) {
	agent, workload := s.ctx.UnitStatus(context.Background(), s.unit, agentStatus, workloadStatus)
	c.Check(agent, jc.DeepEquals, status.StatusInfo{
		Status:  status.Lost,
		Message: "agent is not communicating with the server",
	})
	c.Check(workload, jc.DeepEquals, status.StatusInfo{
		Status:  status.Unknown,
		Message: "agent lost, see 'juju show-status-log foo/2'",
	})
}

func (s *UnitStatusSuite) TestNormal(c *gc.C) {
	s.checkUntouched(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestCAASNormal(c *gc.C) {
	s.ctx.Presence = agentAlive(names.NewApplicationTag(s.unit.app).String())
	s.checkUntouched(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestLost(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.checkLost(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestCAASLost(c *gc.C) {
	s.ctx.Presence = agentDown(names.NewApplicationTag(s.unit.app).String())
	s.checkLost(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestLostTerminated(c *gc.C) {
	workloadStatus := status.StatusInfo{
		Status:  status.Terminated,
		Message: "",
	}

	s.ctx.Presence = agentDown(s.unit.Tag().String())

	agent, workload := s.ctx.UnitStatus(context.Background(), s.unit, agentStatus, workloadStatus)
	c.Check(agent.Status, jc.DeepEquals, status.StatusInfo{
		Status:  status.Lost,
		Message: "agent is not communicating with the server",
	})
	c.Check(workload.Status, jc.DeepEquals, status.StatusInfo{
		Status:  status.Terminated,
		Message: "",
	})
}

func (s *UnitStatusSuite) TestCAASLostTerminated(c *gc.C) {
	wokloadStatus := status.StatusInfo{
		Status:  status.Terminated,
		Message: "",
	}

	s.ctx.Presence = agentDown(names.NewApplicationTag(s.unit.app).String())

	agent, workload := s.ctx.UnitStatus(context.Background(), s.unit, agentStatus, wokloadStatus)
	c.Check(agent.Status, jc.DeepEquals, status.StatusInfo{
		Status:  status.Lost,
		Message: "agent is not communicating with the server",
	})
	c.Check(workload.Status, jc.DeepEquals, status.StatusInfo{
		Status:  status.Terminated,
		Message: "",
	})
}

func (s *UnitStatusSuite) TestLostAndDead(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.unit.life = state.Dead
	// Status is untouched if unit is Dead.
	s.checkUntouched(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestPresenceError(c *gc.C) {
	s.ctx.Presence = presenceError(s.unit.Tag().String())
	// Presence error gets ignored, so no output is unchanged.
	s.checkUntouched(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestNotLostIfAllocating(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	agentStatus := status.StatusInfo{
		Status:  status.Allocating,
		Message: "allocating",
	}
	s.checkUntouched(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestCantBeLostDuringInstall(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	agentStatus := status.StatusInfo{
		Status:  status.Executing,
		Message: "running install hook",
	}
	s.checkUntouched(c, agentStatus, workloadStatus)
}

func (s *UnitStatusSuite) TestCantBeLostDuringWorkloadInstall(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	workloadStatus := status.StatusInfo{
		Status:  status.Maintenance,
		Message: "installing charm software",
	}
	s.checkUntouched(c, agentStatus, workloadStatus)
}

type fakeStatusUnit struct {
	app      string
	presence bool
	life     state.Life
}

func (u *fakeStatusUnit) Name() string {
	return u.app + "/2"
}

func (u *fakeStatusUnit) Tag() names.Tag {
	return names.NewUnitTag(u.Name())
}

func (u *fakeStatusUnit) Life() state.Life {
	return u.life
}
