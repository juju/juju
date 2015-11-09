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

// SetNewComponent sets the component on the registry.
func (c *Components) SetNewComponent(name string, stub *testing.Stub) *Component {
	info := &Component{
		Name: name,
	}

	compCtx := NewContextComponent(stub, info)
	c.SetComponent(name, compCtx)
	return info
}

// ContextComponents is a test double for jujuc.ContextComponents.
type ContextComponents struct {
	contextBase
	info *Components
}

// ContextComponents implements jujuc.ContextComponents.
func (cc ContextComponents) Component(name string) (jujuc.ContextComponent, error) {
	cc.stub.AddCall("Component", name)
	if err := cc.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	component, found := cc.info.Components[name]
	if !found {
		return nil, errors.NotFoundf("component %q", name)
	}
	return component, nil
}

// Component holds the values for the hook context.
type Component struct {
	Name string
}

// ContextComponent is a test double for jujuc.ContextComponent.
type ContextComponent struct {
	contextBase
	info *Component
}

func NewContextComponent(stub *testing.Stub, info *Component) *ContextComponent {
	compCtx := &ContextComponent{
		info: info,
	}
	compCtx.stub = stub
	return compCtx
}

// Get implements jujuc.ContextComponent.
func (cc ContextComponent) Get(name string, result interface{}) error {
	cc.stub.AddCall("Get", name, result)
	if err := cc.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Set implements jujuc.ContextComponent.
func (cc ContextComponent) Set(name string, value interface{}) error {
	cc.stub.AddCall("Set", name, value)
	if err := cc.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Flush implements jujuc.ContextComponent.
func (cc ContextComponent) Flush() error {
	cc.stub.AddCall("Flush")
	if err := cc.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
