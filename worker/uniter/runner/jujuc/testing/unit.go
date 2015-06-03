// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Unit holds the values for the hook context.
type Unit struct {
	Name           string
	Status         jujuc.StatusInfo
	OwnerTag       string
	ConfigSettings charm.Settings
}

// ContextUnit is a test double for jujuc.ContextUnit.
type ContextUnit struct {
	Stub *testing.Stub
	Info *Unit
}

// UnitName implements jujuc.ContextUnit.
func (cu *ContextUnit) UnitName() string {
	cu.Stub.AddCall("UnitName")
	cu.Stub.NextErr()

	return cu.Info.Name
}

// UnitStatus implements jujuc.ContextUnit.
func (cu *ContextUnit) UnitStatus() (*jujuc.StatusInfo, error) {
	cu.Stub.AddCall("UnitStatus")
	if err := cu.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return &cu.Info.Status, nil
}

// SetUnitStatus implements jujuc.ContextUnit.
func (cu *ContextUnit) SetUnitStatus(status jujuc.StatusInfo) error {
	cu.Stub.AddCall("SetUnitStatus", status)
	if err := cu.Stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	cu.Info.Status = status
	return nil
}

// OwnerTag implements jujuc.ContextUnit.
func (cu *ContextUnit) OwnerTag() string {
	cu.Stub.AddCall("OwnerTag")
	cu.Stub.NextErr()

	return cu.Info.OwnerTag
}

// ConfigSettings implements jujuc.ContextUnit.
func (cu *ContextUnit) ConfigSettings() (charm.Settings, error) {
	cu.Stub.AddCall("ConfigSettings")
	if err := cu.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return cu.Info.ConfigSettings, nil
}
