// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/juju/overlord/state"
)

// doesSchemaTableExist return whether the schema table is present in the
// database.
func doesSchemaTableExist(ctx context.Context, tx state.Txn) (bool, error) {
	statement := `
SELECT COUNT(name) FROM sqlite_master WHERE type = 'table' AND name = 'schema'
`
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return false, err
	}
	defer rows.Close()

	if !rows.Next() {
		return false, errors.Errorf("schema table query returned no rows")
	}

	var count int
	if err := rows.Scan(&count); err != nil {
		return false, err
	}

	return count == 1, nil
}

// Create the schema table.
func createSchemaTable(ctx context.Context, tx state.Txn) error {
	statement := `
CREATE TABLE schema (
    id         INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    version    INTEGER NOT NULL,
    updated_at DATETIME NOT NULL,
    UNIQUE (version)
)
`
	_, err := tx.ExecContext(ctx, statement)
	return err
}

// Return the highest update version currently applied. Zero means that no
// updates have been applied yet.
func queryCurrentVersion(ctx context.Context, tx state.Txn) (int, error) {
	versions, err := selectSchemaVersions(ctx, tx)
	if err != nil {
		return -1, errors.Errorf("failed to fetch update versions: %v", err)
	}

	var current int
	if len(versions) > 0 {
		err = checkSchemaVersionsHaveNoHoles(versions)
		if err != nil {
			return -1, err
		}
		current = versions[len(versions)-1] // Highest recorded version
	}

	return current, nil
}

// Return all versions in the schema table, in increasing order.
func selectSchemaVersions(ctx context.Context, tx state.Txn) ([]int, error) {
	statement := `
SELECT version FROM schema ORDER BY version
`
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer rows.Close()

	values := []int{}
	for rows.Next() {
		var value int
		err := rows.Scan(&value)
		if err != nil {
			return nil, errors.Trace(err)
		}
		values = append(values, value)
	}

	err = rows.Err()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return values, nil
}

// Check that the given list of update version numbers doesn't have "holes",
// that is each version equal the preceding version plus 1.
func checkSchemaVersionsHaveNoHoles(versions []int) error {
	// Ensure that there are no "holes" in the recorded versions.
	for i := range versions[:len(versions)-1] {
		if versions[i+1] != versions[i]+1 {
			return errors.Errorf("missing updates: %d to %d", versions[i], versions[i+1])
		}
	}
	return nil
}

// Ensure that the schema exists.
func ensureSchemaTableExists(ctx context.Context, tx state.Txn) error {
	exists, err := doesSchemaTableExist(ctx, tx)
	if err != nil {
		return errors.Errorf("failed to check if schema table is there: %v", err)
	}
	if !exists {
		err := createSchemaTable(ctx, tx)
		if err != nil {
			return errors.Errorf("failed to create schema table: %v", err)
		}
	}
	return nil
}

// Apply any pending update that was not yet applied.
func ensureUpdatesAreApplied(ctx context.Context, tx state.Txn, current int, updates []Update) error {
	if current > len(updates) {
		return errors.Errorf(
			"schema version '%d' is more recent than expected '%d'",
			current, len(updates))
	}

	// If there are no updates, there's nothing to do.
	if len(updates) == 0 {
		return nil
	}

	// Apply missing updates.
	for _, update := range updates[current:] {
		if err := update(tx); err != nil {
			return errors.Errorf("failed to apply update %d: %v", current, err)
		}
		current++

		if err := insertSchemaVersion(ctx, tx, current); err != nil {
			return errors.Errorf("failed to insert version %d", current)
		}
	}

	return nil
}

// Insert a new version into the schema table.
func insertSchemaVersion(ctx context.Context, tx state.Txn, new int) error {
	statement := `
INSERT INTO schema (version, updated_at) VALUES (?, strftime("%s"))
`
	_, err := tx.ExecContext(ctx, statement, new)
	return err
}
