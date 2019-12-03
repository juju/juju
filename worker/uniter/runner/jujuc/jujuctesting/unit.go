// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
)

// Unit holds the values for the hook context.
type Unit struct {
	Name           string
	ConfigSettings charm.Settings
	GoalState      application.GoalState
	PodSpec        string
	CloudSpec      params.CloudSpec
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

// GoalState implements jujuc.ContextUnit.
func (c *ContextUnit) GoalState() (*application.GoalState, error) {
	c.stub.AddCall("GoalState")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return &c.info.GoalState, nil
}

func (c *ContextUnit) SetPodSpec(specYaml string) error {
	c.stub.AddCall("SetPodSpec", specYaml)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}
	c.info.PodSpec = specYaml
	return nil
}

func (c *ContextUnit) GetPodSpec() (string, error) {
	c.stub.AddCall("GetPodSpec")
	if err := c.stub.NextErr(); err != nil {
		return c.info.PodSpec, errors.Trace(err)
	}
	return c.info.PodSpec, nil
}

func (c *ContextUnit) CloudSpec() (*params.CloudSpec, error) {
	c.stub.AddCall("CloudSpec")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	c.info.CloudSpec = params.CloudSpec{}
	return &c.info.CloudSpec, nil
}
