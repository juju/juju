// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain"
	domainsequence "github.com/juju/juju/domain/sequence"
	"github.com/juju/juju/internal/errors"
)

type sequence struct {
	Namespace string `db:"namespace"`
	Value     uint64 `db:"value"`
}

// NextValue returns a monotonically incrementing int value for the given
// namespace.
// The first such value starts at 0.
func NextValue(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX, namespace domainsequence.Namespace) (uint64, error) {
	return incrementSequence(ctx, preparer, tx, 1, namespace)
}

type count struct {
	N uint64 `db:"n"`
}

// NextNValues returns the next n monotonically incrementing uint64 values for
// the given namespace. The values returned by this function will never be
// available for use again.
//
// If zero is supplied to this func no values will be created with no error
// returned.
func NextNValues(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	n uint64,
	namespace domainsequence.Namespace,
) ([]uint64, error) {
	if n == 0 {
		return nil, nil
	}

	latest, err := incrementSequence(ctx, preparer, tx, n, namespace)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Be careful not to shoot ourselves in the foot.
	if latest+1 < n {
		return nil, errors.Errorf("cannot reticulate splines")
	}
	start := latest + 1
	start -= n

	rval := make([]uint64, 0, n)
	for i := range n {
		id := start + i
		rval = append(rval, id)
	}
	return rval, nil
}

// incrementSequence allocates n values and returns the last allocated value.
// Callers must supply a positive n.
func incrementSequence(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	n uint64,
	namespace domainsequence.Namespace,
) (uint64, error) {
	seq := sequence{
		Namespace: namespace.String(),
		Value:     n - 1,
	}
	cnt := count{N: n}

	// Avoid RETURNING: dqlite can mask retryable write conflicts (see
	// https://github.com/canonical/dqlite/pull/918), and SQLair can lose row
	// iteration errors on SQLite. Review this workaround when both are fixed.
	updateStmt, err := preparer.Prepare(`
INSERT INTO sequence (*) VALUES ($sequence.*)
ON CONFLICT (namespace) DO UPDATE SET value = sequence.value + $count.n
`, seq, cnt)
	if err != nil {
		return 0, errors.Capture(err)
	}

	selectStmt, err := preparer.Prepare(`
SELECT s.value AS &sequence.value
FROM sequence AS s
WHERE s.namespace = $sequence.namespace
`, seq)
	if err != nil {
		return 0, errors.Capture(err)
	}

	var outcome sqlair.Outcome
	if err := tx.Query(ctx, updateStmt, seq, cnt).Get(&outcome); err != nil {
		return 0, errors.Errorf("updating sequence number for namespace %q: %w", namespace, err)
	}
	affected, err := outcome.Result().RowsAffected()
	if err != nil {
		return 0, errors.Errorf("getting affected rows for sequence number for namespace %q: %w", namespace, err)
	}
	if affected != 1 {
		return 0, errors.Errorf("updating sequence number for namespace %q: affected %d rows", namespace, affected)
	}

	if err := tx.Query(ctx, selectStmt, seq).Get(&seq); err != nil {
		return 0, errors.Errorf("reading sequence number for namespace %q: %w", namespace, err)
	}
	return seq.Value, nil
}
