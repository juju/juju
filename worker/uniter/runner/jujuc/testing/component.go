// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// Components holds the values for the hook context.
type Components struct {
	Components map[string]*ContextComponent
}

// SetDefault returns the named component if found or adds a new one and
// returns it.
func (c *Components) SetDefault(name string) *ContextComponent {
	if component, ok := c.Components[name]; ok {
		return component
	}
	component := &ContextComponent{}
	c.Components[name] = component
	return component
}

// ContextComponents is a test double for jujuc.ContextComponents.
type ContextComponents struct {
	Stub       *testing.Stub
	Components *Components
}

// ContextComponents implements jujuc.ContextComponents.
func (cc ContextComponents) Component(name string) (jujuc.ContextComponent, bool) {
	cc.Stub.AddCall("Component", name)
	cc.Stub.NextErr()

	component, found := cc.Components.Components[name]
	return *component, found
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
