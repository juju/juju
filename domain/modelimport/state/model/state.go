// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// State provides persistence functionality necessary to import model data.
type State struct {
	*domain.StateBase
}

// NewState returns a new [State] object using the input transaction runner
// factory.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
