// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	"database/sql"
	
	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

type sequence struct {
	Namespace string `db:"namespace"`
	Next      uint   `db:"next_value"`
}

// NextSequenceValue returns a monotonically incrementing int value for the given namespace.
// The first such value starts at 0.
func NextSequenceValue(ctx context.Context, preparer Preparer, tx *sqlair.TX, namespace string) (uint, error) {
	seq := sequence{
		Namespace: namespace,
	}
	updateStmt, err := preparer.Prepare(`
INSERT INTO sequence (namespace, next_value) VALUES ($sequence.namespace, 0)
ON CONFLICT DO UPDATE SET next_value = next_value + 1
`, seq)
	if err != nil {
		return 0, errors.Capture(err)
	}
	
	nextStmt, err := preparer.Prepare(`
SELECT &sequence.next_value FROM sequence WHERE namespace = $sequence.namespace
`, seq)
	if err != nil {
		return 0, errors.Capture(err)
	}

	// Increment the sequence number.
	err = tx.Query(ctx, updateStmt, seq).Run()
	if err != nil {
		return 0, errors.Errorf("updating sequence number for namespace %q: %w", namespace, err)
	}
	// Read the new value to return.
	err = tx.Query(ctx, nextStmt, seq).Get(&seq)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, errors.Errorf("reading sequence number for namespace %q: %w", namespace, err)
	}
	result := seq.Next
	seq.Next++
	return result, nil
}
