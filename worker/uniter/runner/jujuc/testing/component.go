// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Components holds the values for the hook context.
type Components struct {
	Components map[string]jujuc.ContextComponent
}

// SetComponent sets the component on the registry.
func (c *Components) SetComponent(name string, comp jujuc.ContextComponent) {
	if c.Components == nil {
		c.Components = make(map[string]jujuc.ContextComponent)
	}
	c.Components[name] = comp
}

// ContextComponents is a test double for jujuc.ContextComponents.
type ContextComponents struct {
	Stub *testing.Stub
	Info *Components
}

func (c *ContextComponents) init() {
	if c.Stub == nil {
		c.Stub = &testing.Stub{}
	}
	if c.Info == nil {
		c.Info = &Components{}
	}
}

func (c *ContextComponents) setComponent(name string, comp jujuc.ContextComponent) {
	c.init()
	c.Info.SetComponent(name, comp)
}

// ContextComponents implements jujuc.ContextComponents.
func (cc ContextComponents) Component(name string) (jujuc.ContextComponent, error) {
	cc.Stub.AddCall("Component", name)
	if err := cc.Stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	cc.init()
	component, found := cc.Info.Components[name]
	if !found {
		return nil, errors.NotFoundf("component %q", name)
	}
	return component, nil
}

// ContextComponent is a test double for jujuc.ContextComponent.
type ContextComponent struct {
	*testing.Stub
}

// Get implements jujuc.ContextComponent.
func (cc ContextComponent) Get(name string, result interface{}) error {
	cc.AddCall("Get", name, result)
	return cc.NextErr()
}

// Set implements jujuc.ContextComponent.
func (cc ContextComponent) Set(name string, value interface{}) error {
	cc.AddCall("Set", name, value)
	return cc.NextErr()
}

// Flush implements jujuc.ContextComponent.
func (cc ContextComponent) Flush() error {
	cc.AddCall("Flush")
	return cc.NextErr()
}
