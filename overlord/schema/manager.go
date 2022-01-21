// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"

	"github.com/juju/errors"
	"github.com/juju/juju/overlord/state"
)

type State interface {
	DB() *sql.DB
	BeginTx(context.Context) (state.TxnRunner, error)
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
