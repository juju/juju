// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"github.com/juju/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// ServiceLocators holds the values for the hook sub-context.
type ServiceLocators struct {
	ServiceLocators []jujuc.ServiceLocator
}

// AddServiceLocator adds a Service Locator for the provided data.
func (m *ServiceLocators) AddServiceLocator(locators params.AddServiceLocators) {
	for _, sl := range locators.ServiceLocators {
		m.ServiceLocators = append(m.ServiceLocators, jujuc.ServiceLocator{
			Type:   sl.Type,
			Name:   sl.Name,
			Params: sl.Params,
		})
	}

}

// ContextServiceLocators is a test double for jujuc.ContextServiceLocators.
type ContextServiceLocators struct {
	contextBase
	info *ServiceLocators
}

// AddServiceLocator implements jujuc.ContextServiceLocators.
func (c *ContextServiceLocators) AddServiceLocator(locators params.AddServiceLocators) (params.StringResult, error) {
	c.stub.AddCall("AddServiceLocator", locators)
	if err := c.stub.NextErr(); err != nil {
		return params.StringResult{}, errors.Trace(err)
	}

	c.info.AddServiceLocator(locators)
	return params.StringResult{}, nil
}
