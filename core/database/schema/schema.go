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

// Schema captures the schema of a database as a series of ordered updates.
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

// MakePatch returns a patch that applies the input
// statement with the input arguments.
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

// Hook is a callback that gets fired before an update gets applied.
// It allows mutation of the DDL about to be run.
type Hook func(int, string) (string, error)

// New creates a new [Schema] with the input patches.
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

// Hook instructs the schema to invoke the given function whenever an
// update is about to be applied. The function gets passed the update
// version number and the DDL that will be run.
// It returns a modified DDL that will be run instead, and an error.
// A non-nil error will cause the schema transaction to be rolled back.
// Any previously installed hook will be replaced.
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
// All updates are applied transactionally. If an error occurs, the
// transaction will be rolled back and the database will remain unchanged.
//
// An update will be applied only if it hasn't been current (currently applied
// updates are tracked in the 'schema' table, which gets automatically
// created).
//
// If no error occurs, the integer returned by this method is the
// initial version that the schema has been upgraded from.
func (s *Schema) Ensure(ctx context.Context, runner database.TxnRunner) (ChangeSet, error) {
	current, post := -1, -1

	// Make a copy of the patches and apply the hook to each statement.
	// We want to do this before computing hashes.
	toApply := make([]Patch, len(s.patches))
	copy(toApply, s.patches)
	for i, patch := range toApply {
		var err error
		toApply[i].stmt, err = s.hook(i, patch.stmt)
		if err != nil {
			return ChangeSet{}, errors.Errorf("applying hook for patch %d: %w", i, err)
		}
	}

	hashes := computeHashes(toApply)

	err := runner.StdTxn(ctx, func(ctx context.Context, t *sql.Tx) error {
		if err := createSchemaTable(ctx, t); err != nil {
			return errors.Capture(err)
		}

		var err error
		if current, err = validateCurrentVersion(ctx, t, hashes); err != nil {
			return errors.Errorf("querying current schema version: %w", err)
		}

		if err := ensurePatchesAreApplied(ctx, t, current, s.patches, hashes); err != nil {
			return errors.Errorf("applying schema patches: %w", err)
		}

		if post, err = validateCurrentVersion(ctx, t, hashes); err != nil {
			return errors.Errorf("querying post schema version: %w", err)
		}

		return nil
	})

	return ChangeSet{
		Current: current,
		Post:    post,
	}, errors.Capture(err)
}

// omitHook is a no-op hook that does not modify the DDL.
func omitHook(_ int, ddl string) (string, error) { return ddl, nil }
