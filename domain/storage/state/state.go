// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// State represents database interactions dealing with storage.
type State struct {
	*domain.StateBase
}

// NewState returns a new storage state
// based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
