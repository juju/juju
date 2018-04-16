// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Context implements facade.Context in the simplest possible way.
type Context struct {
	Auth_      facade.Authorizer
	Dispose_   func()
	Resources_ facade.Resources
	State_     *state.State
	StatePool_ *state.StatePool
	ID_        string
	// Identity is not part of the facade.Context interface, but is instead
	// used to make sure that the context objects are the same.
	Identity string
}

// Auth is part of the facade.Context interface.
func (context Context) Auth() facade.Authorizer {
	return context.Auth_
}

// Dispose is part of the facade.Context interface.
func (context Context) Dispose() {
	context.Dispose_()
}

// Resources is part of the facade.Context interface.
func (context Context) Resources() facade.Resources {
	return context.Resources_
}

// State is part of the facade.Context interface.
func (context Context) State() *state.State {
	return context.State_
}

// StatePool is part of of the facade.Context interface.
func (context Context) StatePool() *state.StatePool {
	return context.StatePool_
}

// ID is part of the facade.Context interface.
func (context Context) ID() string {
	return context.ID_
}

// Presence implements facade.Context.
func (context Context) Presence() facade.Presence {
	return context
}

// ModelPresence implements facade.Presence.
func (context Context) ModelPresence(modelUUID string) facade.ModelPresence {
	// Potentially may need to add stuff here at some stage.
	return nil
}
