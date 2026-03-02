// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// checkMachinesExist is a transaction based check to assert that the provide
// machine uuids exist in the model. It is the callers responsibility to ensure
// that the supplied machineUUIDs have been de-duplicated. Not doing so can
// result in false return value.
func checkMachinesExist(
	ctx context.Context,
	preparer domain.Preparer,
	tx *sqlair.TX,
	uuids machineUUIDs,
) (bool, error) {
	if len(uuids) == 0 {
		return true, nil
	}

	var foundCount count
	checkQ := `
SELECT COUNT(uuid) AS &count.count
FROM   machine
WHERE  uuid IN ($machineUUIDs[:])
`

	stmt, err := preparer.Prepare(checkQ, uuids, foundCount)
	if err != nil {
		return false, errors.Errorf(
			"preparing check machines exist statement: %w", err,
		)
	}

	err = tx.Query(ctx, stmt, uuids).Get(&foundCount)
	if err != nil {
		return false, errors.Errorf(
			"checking supplied machines exist in model: %w", err,
		)
	}

	return len(uuids) == foundCount.Count, nil
}
