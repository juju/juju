// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Status holds the values for the hook context.
type Status struct {
	UnitStatus    jujuc.StatusInfo
	ServiceStatus jujuc.ServiceStatusInfo
}

// ContextStatus is a test double for jujuc.ContextStatus.
type ContextStatus struct {
	Stub *testing.Stub
	Info *Status
}

func (cu *ContextStatus) init() {
	if cu.Stub == nil {
		cu.Stub = &testing.Stub{}
	}
	if cu.Info == nil {
		cu.Info = &Status{}
	}
}

func (cu *ContextStatus) SetTestingServiceStatus(service jujuc.StatusInfo, units []jujuc.StatusInfo) {
	cu.init()
	cu.Info.ServiceStatus = jujuc.ServiceStatusInfo{
		Service: service,
		Units:   units,
	}
}

// UnitStatus implements jujuc.ContextStatus.
func (cu *ContextStatus) UnitStatus() (*jujuc.StatusInfo, error) {
	cu.Stub.AddCall("UnitStatus")
	if err := cu.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	cu.init()
	return &cu.Info.UnitStatus, nil
}

// SetUnitStatus implements jujuc.ContextStatus.
func (cu *ContextStatus) SetUnitStatus(status jujuc.StatusInfo) error {
	cu.Stub.AddCall("SetUnitStatus", status)
	if err := cu.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	cu.init()
	cu.Info.UnitStatus = status
	return nil
}

// ServiceStatus implements jujuc.ContextStatus.
func (cu *ContextStatus) ServiceStatus() (jujuc.ServiceStatusInfo, error) {
	cu.Stub.AddCall("ServiceStatus")
	if err := cu.Stub.NextErr(); err != nil {
		return jujuc.ServiceStatusInfo{}, errors.Trace(err)
	}

	cu.init()
	return cu.Info.ServiceStatus, nil
}

// SetServiceStatus implements jujuc.ContextStatus.
func (cu *ContextStatus) SetServiceStatus(status jujuc.StatusInfo) error {
	cu.Stub.AddCall("SetServiceStatus", status)
	if err := cu.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	cu.init()
	cu.Info.ServiceStatus = jujuc.ServiceStatusInfo{
		Service: status,
		Units:   []jujuc.StatusInfo{},
	}
	return nil
}
