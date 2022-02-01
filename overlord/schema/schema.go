// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/overlord/state"
)

// Schema captures the schema of a database in terms of a series of ordered
// updates.
type Schema struct {
	updates []Update
}

// Update applies a specific schema change to a database, and returns an error
// if anything goes wrong.
type Update func(state.Txn) error

// New creates a new schema Schema with the given updates.
func New(updates []Update) *Schema {
	return &Schema{
		updates: updates,
	}
}

// Empty creates a new schema with no updates.
func Empty() *Schema {
	return New([]Update{})
}

// Add a new update to the schema. It will be appended at the end of the
// existing series.
func (s *Schema) Add(update Update) {
	s.updates = append(s.updates, update)
}

// Ensure makes sure that the actual schema in the given database matches the
// one defined by our updates.
//
// All updates are applied transactionally. In case any error occurs the
// transaction will be rolled back and the database will remain unchanged.
//
// A update will be applied only if it hasn't been before (currently applied
// updates are tracked in the a 'schema' table, which gets automatically
// created).
//
// If no error occurs, the integer returned by this method is the
// initial version that the schema has been upgraded from.
func (s *Schema) Ensure(st State) (int, error) {
	var current int

	err := st.Run(func(ctx context.Context, t state.Txn) error {
		err := ensureSchemaTableExists(ctx, t)
		if err != nil {
			return errors.Trace(err)
		}

		current, err = queryCurrentVersion(ctx, t)
		if err != nil {
			return errors.Trace(err)
		}

		err = ensureUpdatesAreApplied(ctx, t, current, s.updates)
		if err != nil {
			return errors.Trace(err)
		}

		return nil
	})
	if err != nil {
		return -1, errors.Trace(err)
	}
	return current, nil
}
