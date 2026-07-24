// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/unitless"
)

// State provides persistence operations for unitless applications.
type State struct {
	*domain.StateBase
}

// NewState returns a new unitless state using the supplied transaction runner
// factory.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetApplicationScriptlet returns the scriptlet associated with an
// application.
func (st *State) GetApplicationScriptlet(
	context.Context,
	string,
) (unitless.Scriptlet, error) {
	return unitless.Scriptlet{}, nil
}

// GetScriptletEvent returns the event payload for an application event.
func (st *State) GetScriptletEvent(
	context.Context,
	string,
	string,
) (unitless.Event, error) {
	return unitless.Event{}, nil
}
