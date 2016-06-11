// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Context implements facade.Context in the simplest possible way.
type Context struct {
	Abort_     <-chan struct{}
	Auth_      facade.Authorizer
	Resources_ facade.Resources
	State_     *state.State
	ID_        string
}

// Abort is part of the facade.Context interface.
func (context Context) Abort() <-chan struct{} {
	return context.Abort_
}

// Auth is part of the facade.Context interface.
func (context Context) Auth() facade.Authorizer {
	return context.Auth_
}

// Resources is part of the facade.Context interface.
func (context Context) Resources() facade.Resources {
	return context.Resources_
}

// State is part of the facade.Context interface.
func (context Context) State() *state.State {
	return context.State_
}

// ID is part of the facade.Context interface.
func (context Context) ID() string {
	return context.ID_
}
