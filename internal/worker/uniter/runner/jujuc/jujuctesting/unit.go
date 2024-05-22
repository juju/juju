// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/rpc/params"
)

// Unit holds the values for the hook context.
type Unit struct {
	Name           string
	ConfigSettings charm.Settings
	GoalState      application.GoalState
	K8sSpec        string
	RawK8sSpec     string
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
	_ = c.stub.NextErr()

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
func (c *ContextUnit) GoalState(context.Context) (*application.GoalState, error) {
	c.stub.AddCall("GoalState")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	return &c.info.GoalState, nil
}

func (c *ContextUnit) CloudSpec(context.Context) (*params.CloudSpec, error) {
	c.stub.AddCall("CloudSpec")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}
	c.info.CloudSpec = params.CloudSpec{}
	return &c.info.CloudSpec, nil
}
