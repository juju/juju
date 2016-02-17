// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/status"
)

type ServiceStatusSuite struct {
	ConnSuite
	service *state.Service
}

var _ = gc.Suite(&ServiceStatusSuite{})

func (s *ServiceStatusSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
	s.service = s.Factory.MakeService(c, nil)
}

func (s *ServiceStatusSuite) TestInitialStatus(c *gc.C) {
	s.checkInitialStatus(c)
}

func (s *ServiceStatusSuite) checkInitialStatus(c *gc.C) {
	statusInfo, err := s.service.Status()
	c.Check(err, jc.ErrorIsNil)
	checkInitialWorkloadStatus(c, statusInfo)
}

func (s *ServiceStatusSuite) TestSetUnknownStatus(c *gc.C) {
	err := s.service.SetStatus(status.Status("vliegkat"), "orville", nil)
	c.Check(err, gc.ErrorMatches, `cannot set invalid status "vliegkat"`)

	s.checkInitialStatus(c)
}

func (s *ServiceStatusSuite) TestSetOverwritesData(c *gc.C) {
	err := s.service.SetStatus(status.StatusActive, "healthy", map[string]interface{}{
		"pew.pew": "zap",
	})
	c.Check(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ServiceStatusSuite) TestGetSetStatusAlive(c *gc.C) {
	s.checkGetSetStatus(c)
}

func (s *ServiceStatusSuite) checkGetSetStatus(c *gc.C) {
	err := s.service.SetStatus(status.StatusActive, "healthy", map[string]interface{}{
		"$ping": map[string]interface{}{
			"foo.bar": 123,
		},
	})
	c.Check(err, jc.ErrorIsNil)

	service, err := s.State.Service(s.service.Name())
	c.Assert(err, jc.ErrorIsNil)

	statusInfo, err := service.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(statusInfo.Status, gc.Equals, status.StatusActive)
	c.Check(statusInfo.Message, gc.Equals, "healthy")
	c.Check(statusInfo.Data, jc.DeepEquals, map[string]interface{}{
		"$ping": map[string]interface{}{
			"foo.bar": 123,
		},
	})
	c.Check(statusInfo.Since, gc.NotNil)
}

func (s *ServiceStatusSuite) TestGetSetStatusDying(c *gc.C) {
	_, err := s.service.AddUnit()
	c.Assert(err, jc.ErrorIsNil)
	err = s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	s.checkGetSetStatus(c)
}

func (s *ServiceStatusSuite) TestGetSetStatusGone(c *gc.C) {
	err := s.service.Destroy()
	c.Assert(err, jc.ErrorIsNil)

	err = s.service.SetStatus(status.StatusActive, "not really", nil)
	c.Check(err, gc.ErrorMatches, `cannot set status: service not found`)

	statusInfo, err := s.service.Status()
	c.Check(err, gc.ErrorMatches, `cannot get status: service not found`)
	c.Check(statusInfo, gc.DeepEquals, status.StatusInfo{})
}

func (s *ServiceStatusSuite) TestSetStatusSince(c *gc.C) {
	now := time.Now()

	err := s.service.SetStatus(status.StatusMaintenance, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err := s.service.Status()
	c.Assert(err, jc.ErrorIsNil)
	firstTime := statusInfo.Since
	c.Assert(firstTime, gc.NotNil)
	c.Assert(timeBeforeOrEqual(now, *firstTime), jc.IsTrue)

	// Setting the same status a second time also updates the timestamp.
	err = s.service.SetStatus(status.StatusMaintenance, "", nil)
	c.Assert(err, jc.ErrorIsNil)
	statusInfo, err = s.service.Status()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(timeBeforeOrEqual(*firstTime, *statusInfo.Since), jc.IsTrue)
}

func (s *ServiceStatusSuite) TestDeriveStatus(c *gc.C) {
	// NOTE(fwereade): as detailed in the code, this implementation is not sane.
	// The specified behaviour is arguably sane, but the code is in the wrong
	// place.

	// Create a unit with each possible status.
	addUnit := func(status status.Status) *state.Unit {
		unit, err := s.service.AddUnit()
		c.Assert(err, gc.IsNil)
		err = unit.SetStatus(status, "blam", nil)
		c.Assert(err, gc.IsNil)
		return unit
	}
	blockedUnit := addUnit(status.StatusBlocked)
	waitingUnit := addUnit(status.StatusWaiting)
	maintenanceUnit := addUnit(status.StatusMaintenance)
	terminatedUnit := addUnit(status.StatusTerminated)
	activeUnit := addUnit(status.StatusActive)
	unknownUnit := addUnit(status.StatusUnknown)

	// ...and create one with error status by setting it on the agent :-/.
	errorUnit, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = errorUnit.Agent().SetStatus(status.StatusError, "blam", nil)
	c.Assert(err, gc.IsNil)

	// For each status, in order of severity, check the service status is
	// derived from that unit status; then remove that unit and proceed to
	// the next severity.
	checkAndRemove := func(unit *state.Unit, status status.Status) {
		info, err := s.service.Status()
		c.Check(err, jc.ErrorIsNil)
		c.Check(info.Status, gc.Equals, status)

		err = unit.Destroy()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.EnsureDead()
		c.Assert(err, jc.ErrorIsNil)
		err = unit.Remove()
		c.Assert(err, jc.ErrorIsNil)
	}
	checkAndRemove(errorUnit, status.StatusError)
	checkAndRemove(blockedUnit, status.StatusBlocked)
	checkAndRemove(waitingUnit, status.StatusWaiting)
	checkAndRemove(maintenanceUnit, status.StatusMaintenance)
	checkAndRemove(terminatedUnit, status.StatusTerminated)
	checkAndRemove(activeUnit, status.StatusActive)
	checkAndRemove(unknownUnit, status.StatusUnknown)
}

func (s *ServiceStatusSuite) TestServiceStatusOverridesDerivedStatus(c *gc.C) {
	unit, err := s.service.AddUnit()
	c.Assert(err, gc.IsNil)
	err = unit.SetStatus(status.StatusBlocked, "pow", nil)
	c.Assert(err, gc.IsNil)
	err = s.service.SetStatus(status.StatusMaintenance, "zot", nil)
	c.Assert(err, gc.IsNil)

	info, err := s.service.Status()
	c.Check(err, jc.ErrorIsNil)
	c.Check(info.Status, gc.Equals, status.StatusMaintenance)
}
