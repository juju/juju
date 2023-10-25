// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/juju/api/base"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const apiName = "AgentTools"

// Facade provides access to an api used for manipulating agent tools.
type Facade struct {
	facade base.FacadeCaller
}

// NewFacade returns a new api client facade instance.
func NewFacade(caller base.APICaller, options ...Option) *Facade {
	facadeCaller := base.NewFacadeCaller(caller, apiName, options...)
	return &Facade{facadeCaller}
}

// UpdateToolsVersion calls UpdateToolsAvailable in the server.
func (f *Facade) UpdateToolsVersion() error {
	return f.facade.FacadeCall(context.TODO(), "UpdateToolsAvailable", nil, nil)
}
