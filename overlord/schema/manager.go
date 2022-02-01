// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/overlord/state"
)

type State interface {
	// Run is a convince function for running one shot transactions, which
	// correctly handles the rollback semantics and retries where available.
	Run(func(context.Context, state.Txn) error) error
	// CreateTxn creates a transaction builder. The transaction builder
	// accumulates a series of functions that can be executed on a given commit.
	CreateTxn(context.Context) (state.TxnBuilder, error)
}

type SchemaManager struct {
	state  State
	schema *Schema
}

func NewManager(s State, schema *Schema) *SchemaManager {
	mgr := &SchemaManager{
		state:  s,
		schema: schema,
	}
	return mgr
}

func (m *SchemaManager) StartUp(ctx context.Context) error {
	_, err := m.schema.Ensure(m.state)
	return errors.Trace(err)
}

func (m *SchemaManager) Ensure() error {
	return nil
}
