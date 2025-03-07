// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

// Status  holds the values for the hook context.
type Status struct {
	UnitStatus        jujuc.StatusInfo
	ApplicationStatus jujuc.ApplicationStatusInfo
}

// SetApplicationStatus builds a application status and sets it on the Status.
func (s *Status) SetApplicationStatus(application jujuc.StatusInfo, units []jujuc.StatusInfo) {
	s.ApplicationStatus = jujuc.ApplicationStatusInfo{
		Application: application,
		Units:       units,
	}
}

// ContextStatus is a test double for jujuc.ContextStatus.
type ContextStatus struct {
	contextBase
	info *Status
}

// UnitStatus implements jujuc.ContextStatus.
func (c *ContextStatus) UnitStatus() (*jujuc.StatusInfo, error) {
	c.stub.AddCall("UnitStatus")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return &c.info.UnitStatus, nil
}

// SetUnitStatus implements jujuc.ContextStatus.
func (c *ContextStatus) SetUnitStatus(status jujuc.StatusInfo) error {
	c.stub.AddCall("SetUnitStatus", status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.UnitStatus = status
	return nil
}

// ApplicationStatus implements jujuc.ContextStatus.
func (c *ContextStatus) ApplicationStatus() (jujuc.ApplicationStatusInfo, error) {
	c.stub.AddCall("ApplicationStatus")
	if err := c.stub.NextErr(); err != nil {
		return jujuc.ApplicationStatusInfo{}, errors.Trace(err)
	}

	return c.info.ApplicationStatus, nil
}

// SetApplicationStatus implements jujuc.ContextStatus.
func (c *ContextStatus) SetApplicationStatus(status jujuc.StatusInfo) error {
	c.stub.AddCall("SetApplicationStatus", status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.SetApplicationStatus(status, nil)
	return nil
}
