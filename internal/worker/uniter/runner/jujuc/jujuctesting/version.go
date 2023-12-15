// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
)

// Version holds values for the hook context.
type Version struct {
	WorkloadVersion string
}

// ContextVersion is a test double for jujuc.ContextVersion.
type ContextVersion struct {
	contextBase
	info *Version
}

// UnitWorkloadVersion implements jujuc.ContextVersion.
func (c *ContextVersion) UnitWorkloadVersion() (string, error) {
	c.stub.AddCall("UnitWorkloadVersion")
	if err := c.stub.NextErr(); err != nil {
		return "", errors.Trace(err)
	}
	return c.info.WorkloadVersion, nil
}

// SetUnitWorkloadVersion implements jujuc.ContextVersion.
func (c *ContextVersion) SetUnitWorkloadVersion(version string) error {
	c.stub.AddCall("SetUnitWorkloadVersion", version)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}
	c.info.WorkloadVersion = version
	return nil
}
