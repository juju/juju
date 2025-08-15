// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/juju/juju/internal/errors"
)

const schemaTable = `
CREATE TABLE IF NOT EXISTS schema (
    id           INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
    version      INTEGER NOT NULL,
    hash         TEXT NOT NULL,
    updated_at   DATETIME NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_schema_version ON schema (version);
`

// Create the schema table.
func createSchemaTable(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, schemaTable)
	return errors.Capture(err)
}

// validateCurrentVersion checks that the input hashes match the
// hashes of the patches that have been applied to the database.
// It returns the highest patch version currently applied.
// Zero means that no patches have been applied yet.
func validateCurrentVersion(ctx context.Context, tx *sql.Tx, computedHashes []string) (int, error) {
	versions, err := selectSchemaVersions(ctx, tx)
	if err != nil {
		return -1, errors.Errorf("failed to fetch patch versions: %v", err)
	}

	var current int
	if len(versions) > 0 {
		if err := checkSchemaVersionsHaveNoHoles(versions); err != nil {
			return -1, errors.Capture(err)
		}
		if err := checkSchemaHashesMatch(versions, computedHashes); err != nil {
			return -1, errors.Capture(err)
		}

		// Highest recorded version
		current = versions[len(versions)-1].version
	}

	return current, nil
}

type versionHash struct {
	version int
	hash    string
}

// Return all versions in the schema table, in increasing order.
func selectSchemaVersions(ctx context.Context, tx *sql.Tx) ([]versionHash, error) {
	statement := `SELECT version, hash FROM schema ORDER BY version;`
	rows, err := tx.QueryContext(ctx, statement)
	if err != nil {
		return nil, errors.Capture(err)
	}
	defer func() { _ = rows.Close() }()

	var values []versionHash
	for rows.Next() {
		var version versionHash
		err := rows.Scan(&version.version, &version.hash)
		if err != nil {
			return nil, errors.Capture(err)
		}
		values = append(values, version)
	}

	if err := rows.Err(); err != nil {
		return nil, errors.Capture(err)
	}
	return values, nil
}

// Check that the given list of update version numbers doesn't have "holes",
// that is each version equal the preceding version plus 1.
func checkSchemaVersionsHaveNoHoles(versions []versionHash) error {
	// Ensure that there are no "holes" in the recorded versions.
	for i := range versions[:len(versions)-1] {
		if versions[i+1].version != versions[i].version+1 {
			return errors.Errorf("missing patches: %d to %d", versions[i].version, versions[i+1].version)
		}
	}
	return nil
}

func checkSchemaHashesMatch(versions []versionHash, computedHashes []string) error {
	// Ensure that the recorded hashes match the computed hashes.
	for i, version := range versions {
		if version.hash != computedHashes[i] {
			return errors.Errorf("hash mismatch for version %d", version.version)
		}
	}
	return nil
}

// ensurePatchesAreApplied applies any pending patch that was not yet applied.
// It returns an error if the current version is more recent than the
// expected version, or if any patch fails to apply.
// The input hashes are assumed to be valid for the input patches.
func ensurePatchesAreApplied(ctx context.Context, tx *sql.Tx, current int, patches []Patch, hashes []string) error {
	if current > len(patches) {
		return errors.Errorf("schema version '%d' is more recent than expected '%d'", current, len(patches))
	}

	if len(patches) == 0 {
		return nil
	}

	for _, patch := range patches[current:] {
		if err := ctx.Err(); err != nil {
			return errors.Capture(err)
		}

		if err := patch.run(ctx, tx); err != nil {
			return errors.Errorf("failed to apply patch %d: %v", current, err)
		}
		current++

		if err := insertSchemaVersion(ctx, tx, versionHash{
			version: current,
			hash:    hashes[current-1],
		}); err != nil {
			return errors.Errorf("failed to insert version %d", current)
		}
	}

	return nil
}

// Insert a new version into the schema table.
func insertSchemaVersion(ctx context.Context, tx *sql.Tx, new versionHash) error {
	statement := `INSERT INTO schema (version, hash, updated_at) VALUES (?, ?, strftime("%s"));`
	_, err := tx.ExecContext(ctx, statement, new.version, new.hash)
	return errors.Capture(err)
}

const seedHash = "WUBMFo6cr9oo+WDQUqJigD6fH3znpMhWwy/FOwzHfY0="

func computeHashes(patches []Patch) []string {
	hashes := make([]string, len(patches))
	prev := seedHash
	for i, patch := range patches {
		hash := fmt.Sprintf("%s %s", prev, patch.hash)
		hashes[i] = computeHash(hash)
		prev = hash
	}
	return hashes
}

func computeHash(s string) string {
	trimmed := strings.TrimSpace(s)
	sum := sha256.Sum256([]byte(trimmed))
	return base64.StdEncoding.EncodeToString(sum[:])
}
