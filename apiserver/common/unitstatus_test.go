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

type UnitStatusSuite struct {
	testing.IsolationSuite
	unit *fakeStatusUnit
}

var _ = gc.Suite(&UnitStatusSuite{})

func (s *UnitStatusSuite) SetUpTest(c *gc.C) {
	s.unit = &fakeStatusUnit{
		agentStatus: status.StatusInfo{
			Status:  status.Started,
			Message: "agent ok",
		},
		status: status.StatusInfo{
			Status:  status.Idle,
			Message: "unit ok",
		},
		presence: true,
	}
}

func (s *UnitStatusSuite) checkUntouched(c *gc.C) {
	agent, workload := common.UnitStatus(s.unit)
	c.Check(agent.Status, jc.DeepEquals, s.unit.agentStatus)
	c.Check(agent.Err, jc.ErrorIsNil)
	c.Check(workload.Status, jc.DeepEquals, s.unit.status)
	c.Check(workload.Err, jc.ErrorIsNil)
}

func (s *UnitStatusSuite) TestNormal(c *gc.C) {
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestErrors(c *gc.C) {
	s.unit.agentStatusErr = errors.New("agent status error")
	s.unit.statusErr = errors.New("status error")

	agent, workload := common.UnitStatus(s.unit)
	c.Check(agent.Err, gc.ErrorMatches, "agent status error")
	c.Check(workload.Err, gc.ErrorMatches, "status error")
}

func (s *UnitStatusSuite) TestLost(c *gc.C) {
	s.unit.presence = false
	agent, workload := common.UnitStatus(s.unit)
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

func (s *UnitStatusSuite) TestLostAndDead(c *gc.C) {
	s.unit.presence = false
	s.unit.life = state.Dead
	// Status is untouched if unit is Dead.
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestPresenceError(c *gc.C) {
	s.unit.presence = false
	s.unit.presenceErr = errors.New("boom")
	// Presence error gets ignored, so no output is unchanged.
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestNotLostIfAllocating(c *gc.C) {
	s.unit.presence = false
	s.unit.agentStatus.Status = status.Allocating
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestCantBeLostDuringInstall(c *gc.C) {
	s.unit.presence = false
	s.unit.agentStatus.Status = status.Executing
	s.unit.agentStatus.Message = "running install hook"
	s.checkUntouched(c)
}

func (s *UnitStatusSuite) TestCantBeLostDuringWorkloadInstall(c *gc.C) {
	s.unit.presence = false
	s.unit.status.Status = status.Maintenance
	s.unit.status.Message = "installing charm software"
	s.checkUntouched(c)
}

type fakeStatusUnit struct {
	agentStatus    status.StatusInfo
	agentStatusErr error
	status         status.StatusInfo
	statusErr      error
	presence       bool
	presenceErr    error
	life           state.Life
}

func (u *fakeStatusUnit) Name() string {
	return "foo/2"
}

func (u *fakeStatusUnit) AgentStatus() (status.StatusInfo, error) {
	return u.agentStatus, u.agentStatusErr
}

func (u *fakeStatusUnit) Status() (status.StatusInfo, error) {
	return u.status, u.statusErr
}

func (u *fakeStatusUnit) AgentPresence() (bool, error) {
	return u.presence, u.presenceErr
}

func (u *fakeStatusUnit) Life() state.Life {
	return u.life
}
