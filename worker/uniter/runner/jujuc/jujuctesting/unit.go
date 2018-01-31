// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
)

// Unit holds the values for the hook context.
type Unit struct {
	Name           string
	ConfigSettings charm.Settings
	ContainerSpec  string
	Application    bool
}

// ContextUnit is a test double for jujuc.ContextUnit.
type ContextUnit struct {
	contextBase
	info *Unit
}

// UnitName implements jujuc.ContextUnit.
func (c *ContextUnit) UnitName() string {
	c.stub.AddCall("UnitName")
	c.stub.NextErr()

	return c.info.Name
}

// ConfigSettings implements jujuc.ContextUnit.
func (c *ContextUnit) ConfigSettings() (charm.Settings, error) {
	c.stub.AddCall("ConfigSettings")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return c.info.ConfigSettings, nil
}

func (c *ContextUnit) SetContainerSpec(specYaml string, application bool) error {
	c.stub.AddCall("SetContainerSpec", specYaml, application)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}
	c.info.Application = application
	c.info.ContainerSpec = specYaml
	return nil
}
