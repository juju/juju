// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/common/hookcommands"
)

// Status  holds the values for the hook context.
type Status struct {
	UnitStatus        hookcommands.StatusInfo
	ApplicationStatus hookcommands.ApplicationStatusInfo
}

// SetApplicationStatus builds a service status and sets it on the Status.
func (s *Status) SetApplicationStatus(service hookcommands.StatusInfo, units []hookcommands.StatusInfo) {
	s.ApplicationStatus = hookcommands.ApplicationStatusInfo{
		Application: service,
		Units:       units,
	}
}

// ContextStatus is a test double for hooks.ContextStatus.
type ContextStatus struct {
	contextBase
	info *Status
}

// UnitStatus implements hooks.ContextStatus.
func (c *ContextStatus) UnitStatus() (*hookcommands.StatusInfo, error) {
	c.stub.AddCall("UnitStatus")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return &c.info.UnitStatus, nil
}

// SetUnitStatus implements hooks.ContextStatus.
func (c *ContextStatus) SetUnitStatus(status hookcommands.StatusInfo) error {
	c.stub.AddCall("SetUnitStatus", status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.UnitStatus = status
	return nil
}

// ApplicationStatus implements hooks.ContextStatus.
func (c *ContextStatus) ApplicationStatus() (hookcommands.ApplicationStatusInfo, error) {
	c.stub.AddCall("ApplicationStatus")
	if err := c.stub.NextErr(); err != nil {
		return hookcommands.ApplicationStatusInfo{}, errors.Trace(err)
	}

	return c.info.ApplicationStatus, nil
}

// SetApplicationStatus implements hooks.ContextStatus.
func (c *ContextStatus) SetApplicationStatus(status hookcommands.StatusInfo) error {
	c.stub.AddCall("SetApplicationStatus", status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.SetApplicationStatus(status, nil)
	return nil
}
