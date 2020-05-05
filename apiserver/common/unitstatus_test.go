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

type UnitStatusSuite struct {
	testing.IsolationSuite
	ctx  common.ModelPresenceContext
	unit *fakeStatusUnit
}

var _ = gc.Suite(&UnitStatusSuite{})

func (s *UnitStatusSuite) SetUpTest(c *gc.C) {
	s.unit = &fakeStatusUnit{
		app: "foo",
		agentStatus: status.StatusInfo{
			Status:  status.Started,
			Message: "agent ok",
		},
		status: status.StatusInfo{
			Status:  status.Idle,
			Message: "unit ok",
		},
		presence:         true,
		shouldBeAssigned: true,
	}
	s.ctx = common.ModelPresenceContext{
		Presence: agentAlive(names.NewUnitTag(s.unit.Name()).String()),
	}
}

func (s *UnitStatusSuite) checkUntouched(c *gc.C) {
	agent, workload := s.ctx.UnitStatus(s.unit)
	c.Check(agent.Status, jc.DeepEquals, s.unit.agentStatus)
	c.Check(agent.Err, jc.ErrorIsNil)
	c.Check(workload.Status, jc.DeepEquals, s.unit.status)
	c.Check(workload.Err, jc.ErrorIsNil)
}

func (s *UnitStatusSuite) checkLost(c *gc.C) {
	agent, workload := s.ctx.UnitStatus(s.unit)
	c.Check(agent.Status, jc.DeepEquals, status.StatusInfo{
		Status:  status.Lost,
		Message: "agent is not communicating with the server",
	})
	c.Check(agent.Err, jc.ErrorIsNil)
	c.Check(workload.Status, jc.DeepEquals, status.StatusInfo{
		Status:  status.Unknown,
		Message: "agent lost, see 'juju show-status-log foo/2'",
	})
	c.Check(workload.Err, jc.ErrorIsNil)
}

func (s *UnitStatusSuite) TestNormal(c *gc.C) {
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestCAASNormal(c *gc.C) {
	s.unit.shouldBeAssigned = false
	s.ctx.Presence = agentAlive(names.NewApplicationTag(s.unit.app).String())
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestErrors(c *gc.C) {
	s.unit.agentStatusErr = errors.New("agent status error")
	s.unit.statusErr = errors.New("status error")

	agent, workload := s.ctx.UnitStatus(s.unit)
	c.Check(agent.Err, gc.ErrorMatches, "agent status error")
	c.Check(workload.Err, gc.ErrorMatches, "status error")
}

func (s *UnitStatusSuite) TestLost(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.checkLost(c)
}

func (s *UnitStatusSuite) TestCAASLost(c *gc.C) {
	s.unit.shouldBeAssigned = false
	s.ctx.Presence = agentDown(names.NewApplicationTag(s.unit.app).String())
	s.checkLost(c)
}

func (s *UnitStatusSuite) TestLostAndDead(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.unit.life = state.Dead
	// Status is untouched if unit is Dead.
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestPresenceError(c *gc.C) {
	s.ctx.Presence = presenceError(s.unit.Tag().String())
	// Presence error gets ignored, so no output is unchanged.
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestNotLostIfAllocating(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.unit.agentStatus.Status = status.Allocating
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestCantBeLostDuringInstall(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.unit.agentStatus.Status = status.Executing
	s.unit.agentStatus.Message = "running install hook"
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestCantBeLostDuringWorkloadInstall(c *gc.C) {
	s.ctx.Presence = agentDown(s.unit.Tag().String())
	s.unit.status.Status = status.Maintenance
	s.unit.status.Message = "installing charm software"
	s.checkUntouched(c)
}

type fakeStatusUnit struct {
	app              string
	agentStatus      status.StatusInfo
	agentStatusErr   error
	status           status.StatusInfo
	statusErr        error
	presence         bool
	presenceErr      error
	life             state.Life
	shouldBeAssigned bool
}

func (u *fakeStatusUnit) Name() string {
	return u.app + "/2"
}

func (u *fakeStatusUnit) Tag() names.Tag {
	return names.NewUnitTag(u.Name())
}

func (u *fakeStatusUnit) AgentStatus() (status.StatusInfo, error) {
	return u.agentStatus, u.agentStatusErr
}

func (u *fakeStatusUnit) Status() (status.StatusInfo, error) {
	return u.status, u.statusErr
}

func (u *fakeStatusUnit) Life() state.Life {
	return u.life
}

func (u *fakeStatusUnit) ShouldBeAssigned() bool {
	return u.shouldBeAssigned
}
