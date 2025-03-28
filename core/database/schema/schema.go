// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/internal/errors"
)

// Tx describes the ability to execute a SQL statement within a transaction.
type Tx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// Schema captures the schema of a database in terms of a series of ordered
// updates.
type Schema struct {
	patches []Patch
	hook    Hook
}

// Patch applies a specific schema change to a database, and returns an error
// if anything goes wrong.
type Patch struct {
	run  func(context.Context, Tx) error
	hash string
	stmt string
}

// MakePatch returns a patch that applies the given SQL statement with the given
// arguments.
func MakePatch(statement string, args ...any) Patch {
	return Patch{
		run: func(ctx context.Context, tx Tx) error {
			_, err := tx.ExecContext(ctx, statement, args...)
			return errors.Capture(err)
		},
		hash: computeHash(statement),
		stmt: statement,
	}
}

// Hook is a callback that gets fired when a update gets applied.
type Hook func(int, string) error

// New creates a new schema Schema with the given patches.
func New(patches ...Patch) *Schema {
	return &Schema{
		patches: patches,
		hook:    omitHook,
	}
}

// Add a new update to the schema. It will be appended at the end of the
// existing series.
func (s *Schema) Add(patches ...Patch) {
	s.patches = append(s.patches, patches...)
}

// Hook instructs the schema to invoke the given function whenever a update is
// about to be applied. The function gets passed the update version number and
// the running transaction, and if it returns an error it will cause the schema
// transaction to be rolled back. Any previously installed hook will be
// replaced.
func (s *Schema) Hook(hook Hook) {
	s.hook = hook
}

// Len returns the number of total patches in the schema.
func (s *Schema) Len() int {
	return len(s.patches)
}

// ChangeSet returns the schema changes for the schema when they're applied.
type ChangeSet struct {
	Current, Post int
}

// Ensure makes sure that the actual schema in the given database matches the
// one defined by our updates.
//
// All updates are applied transactionally. In case any error occurs the
// transaction will be rolled back and the database will remain unchanged.
//
// A update will be applied only if it hasn't been current (currently applied
// updates are tracked in the a 'schema' table, which gets automatically
// created).
//
// If no error occurs, the integer returned by this method is the
// initial version that the schema has been upgraded from.
func (s *Schema) Ensure(ctx context.Context, runner database.TxnRunner) (ChangeSet, error) {
	current, post := -1, -1
	err := runner.StdTxn(ctx, func(ctx context.Context, t *sql.Tx) error {
		if err := createSchemaTable(ctx, t); err != nil {
			return errors.Capture(err)
		}

		hashes := computeHashes(s.patches)

		var err error
		if current, err = queryCurrentVersion(ctx, t, hashes); err != nil {
			return errors.Errorf("failed to query current schema version: %w", err)
		}

		if err := ensurePatchesAreApplied(ctx, t, current, s.patches, s.hook); err != nil {
			return errors.Errorf("failed to apply schema patches: %w", err)
		}

		if post, err = queryCurrentVersion(ctx, t, hashes); err != nil {
			return errors.Errorf("failed to query post schema version: %w", err)
		}

		return nil
	})
	return ChangeSet{
		Current: current,
		Post:    post,
	}, errors.Capture(err)
}

// omitHook always returns a nil, omitting the error.
func omitHook(int, string) error { return nil }
