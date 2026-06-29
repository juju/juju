// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package model holds the model-DB import state. Import is the write-mirror of
// the export state (domain/export/state/model): it inserts the transformed,
// target-version payload back into the model DB. The Import method itself is
// generated (import.go) from the live model schema, so it stays in lockstep with
// the export types and needs no per-table maintenance.
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
