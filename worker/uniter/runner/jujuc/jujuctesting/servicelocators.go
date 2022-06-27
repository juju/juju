// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// ServiceLocators holds the values for the hook sub-context.
type ServiceLocators struct {
	ServiceLocators []jujuc.ServiceLocator
}

// AddServiceLocator adds a Service Locator for the provided data.
func (m *ServiceLocators) AddServiceLocator(slType string, slName string, slParams map[string]interface{}) {
	m.ServiceLocators = append(m.ServiceLocators, jujuc.ServiceLocator{
		Type:   slType,
		Name:   slName,
		Params: slParams,
	})
}

// ContextServiceLocators is a test double for jujuc.ContextServiceLocators.
type ContextServiceLocators struct {
	contextBase
	info *ServiceLocators
}

// AddServiceLocator implements jujuc.ContextServiceLocators.
func (c *ContextServiceLocators) AddServiceLocator(slType string, slName string, slParams map[string]interface{}) error {
	c.stub.AddCall("AddServiceLocator", slType, slName, slParams)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.info.AddServiceLocator(slType, slName, slParams)
	return nil
}
