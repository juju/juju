// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain"
	domainsequence "github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/errors"
)

// NextValue returns a monotonically incrementing int value for the given namespace.
// The first such value starts at 0.
func NextValue(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX, namespace domainsequence.Namespace) (uint64, error) {
	seq := sequence{
		Namespace: namespace.String(),
	}
	updateStmt, err := preparer.Prepare(`
INSERT INTO sequence (namespace, value) VALUES ($sequence.namespace, 0)
ON CONFLICT DO UPDATE SET value = value + 1
`, seq)
	if err != nil {
		return 0, errors.Capture(err)
	}

	nextStmt, err := preparer.Prepare(`
SELECT &sequence.value FROM sequence WHERE namespace = $sequence.namespace
`, seq)
	if err != nil {
		return 0, errors.Capture(err)
	}

	// Increment the sequence number, we need to ensure that we affected the
	// sequence, otherwise we can end up with values that aren't unique.
	var outcome sqlair.Outcome
	err = tx.Query(ctx, updateStmt, seq).Get(&outcome)
	if err != nil {
		return 0, errors.Errorf("updating sequence number for namespace %q: %w", namespace, err)
	}
	if affected, err := outcome.Result().RowsAffected(); err != nil {
		return 0, errors.Errorf("getting affected rows for sequence number for namespace %q: %w", namespace, err)
	} else if affected != 1 {
		return 0, errors.Errorf("updating sequence number for namespace %q", namespace)
	}

	// Read the new value to return.
	err = tx.Query(ctx, nextStmt, seq).Get(&seq)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, errors.Errorf("reading sequence number for namespace %q: %w", namespace, err)
	}
	result := seq.Value
	return result, nil
}

type sequence struct {
	Namespace string `db:"namespace"`
	Value     uint64 `db:"value"`
}
