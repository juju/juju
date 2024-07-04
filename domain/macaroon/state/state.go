// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/database"
)

// State describes the persistence layer for macaroon bakery
// bakery storage
type State struct {
	*BakeryConfigState
}

// NewState returns a new state reference
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		BakeryConfigState: NewBakeryConfigState(factory),
	}
}
