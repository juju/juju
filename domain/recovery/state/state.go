// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
)

// State supplies persistence for one exclusively owned recovery database.
type State struct {
	*domain.StateBase
}

// NewState constructs persistence using the supplied database factory.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{StateBase: domain.NewStateBase(factory)}
}
