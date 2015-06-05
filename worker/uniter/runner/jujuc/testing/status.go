// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Status  holds the values for the hook context.
type Status struct {
	UnitStatus    jujuc.StatusInfo
	ServiceStatus jujuc.ServiceStatusInfo
}

func (s *Status) SetServiceStatus(service jujuc.StatusInfo, units []jujuc.StatusInfo) {
	s.ServiceStatus = jujuc.ServiceStatusInfo{
		Service: service,
		Units:   units,
	}
}

// ContextStatus is a test double for jujuc.ContextStatus.
type ContextStatus struct {
	Stub *testing.Stub
	Info *Status
}

func (c *ContextStatus) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
	if c.Info == nil {
		c.Info = &Status{}
	}
}

// UnitStatus implements jujuc.ContextStatus.
func (c *ContextStatus) UnitStatus() (*jujuc.StatusInfo, error) {
	c.Stub.AddCall("UnitStatus")
	if err := c.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	c.init()
	return &c.Info.UnitStatus, nil
}

// SetUnitStatus implements jujuc.ContextStatus.
func (c *ContextStatus) SetUnitStatus(status jujuc.StatusInfo) error {
	c.Stub.AddCall("SetUnitStatus", status)
	if err := c.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.init()
	c.Info.UnitStatus = status
	return nil
}

// ServiceStatus implements jujuc.ContextStatus.
func (c *ContextStatus) ServiceStatus() (jujuc.ServiceStatusInfo, error) {
	c.Stub.AddCall("ServiceStatus")
	if err := c.Stub.NextErr(); err != nil {
		return jujuc.ServiceStatusInfo{}, errors.Trace(err)
	}

	c.init()
	return c.Info.ServiceStatus, nil
}

// SetServiceStatus implements jujuc.ContextStatus.
func (c *ContextStatus) SetServiceStatus(status jujuc.StatusInfo) error {
	c.Stub.AddCall("SetServiceStatus", status)
	if err := c.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.init()
	c.Info.ServiceStatus = jujuc.ServiceStatusInfo{
		Service: status,
		Units:   []jujuc.StatusInfo{},
	}
	return nil
}
