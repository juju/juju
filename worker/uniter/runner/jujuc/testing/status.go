// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Status  holds the values for the hook context.
type Status struct {
	UnitStatus    jujuc.StatusInfo
	ServiceStatus jujuc.ServiceStatusInfo
}

// SetServiceStatus builds a service status and sets it on the Status.
func (s *Status) SetServiceStatus(service jujuc.StatusInfo, units []jujuc.StatusInfo) {
	s.ServiceStatus = jujuc.ServiceStatusInfo{
		Service: service,
		Units:   units,
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

// ServiceStatus implements jujuc.ContextStatus.
func (c *ContextStatus) ServiceStatus() (jujuc.ServiceStatusInfo, error) {
	c.stub.AddCall("ServiceStatus")
	if err := c.stub.NextErr(); err != nil {
		return jujuc.ServiceStatusInfo{}, errors.Trace(err)
	}

	return c.info.ServiceStatus, nil
}

// SetServiceStatus implements jujuc.ContextStatus.
func (c *ContextStatus) SetServiceStatus(status jujuc.StatusInfo) error {
	c.stub.AddCall("SetServiceStatus", status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.SetServiceStatus(status, nil)
	return nil
}
