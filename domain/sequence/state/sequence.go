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

	// Create the namespace on first use or increment its stored value in one
	// statement. The allocation remains part of the caller's transaction, so
	// it rolls back if the caller's other writes fail.
	stmt, err := preparer.Prepare(`
INSERT INTO sequence (*) VALUES ($sequence.*)
ON CONFLICT (namespace) DO UPDATE SET value = sequence.value + $count.n
RETURNING &sequence.value
`, seq, cnt)
	if err != nil {
		return 0, errors.Capture(err)
	}

	// The upsert targets one namespace; a returned row confirms it was updated.
	if err := tx.Query(ctx, stmt, seq, cnt).Get(&seq); err != nil {
		return 0, errors.Errorf("updating sequence number for namespace %q: %w", namespace, err)
	}
	return seq.Value, nil
}
