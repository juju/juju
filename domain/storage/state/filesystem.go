// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"slices"

	"github.com/canonical/sqlair"

	coremachine "github.com/juju/juju/core/machine"
	domainmachineerrors "github.com/juju/juju/domain/machine/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// GetFilesystemUUIDsByMachines returns all of the
// [domainstorage.FilesystemUUID]s in the model that are attached to at least
// one of the supplied [coremachine.UUID]s.
//
// Should no Filesystems be attached to any machine in the model an empty slice
// is returned. As well as should an empty list of Machine UUIDs be supplied an
// empty slice is returned with no error.
//
// The following errors may be returned:
// - [domainmachineerrors.MachineNotFound] when one or more the supplied machine
// uuids does not exist in the model.
func (st *State) GetFilesystemUUIDsByMachines(
	ctx context.Context, uuids []coremachine.UUID,
) ([]domainstorage.FilesystemUUID, error) {
	if len(uuids) == 0 {
		// early exit, no machines equals no work to be done
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	// de-dupe machine uuids
	uuids = slices.Compact(uuids)
	machineUUIDInputs := make(machineUUIDs, 0, len(uuids))
	for _, u := range uuids {
		machineUUIDInputs = append(machineUUIDInputs, u.String())
	}

	// It is possible that a filesystem is attached to more then one machine.
	// For that reason we MUST make sure we only return the Filesystem UUID once.
	selectQ := `
SELECT DISTINCT sf.uuid AS (&entityUUID.uuid)
FROM   storage_filesystem sf
JOIN   storage_filesystem_attachment sfa ON sf.uuid = sfa.filesystem_uuid
JOIN   machine m ON sfa.net_node_uuid = m.net_node_uuid
WHERE  m.uuid IN ($machineUUIDs[:])
`

	stmt, err := st.Prepare(selectQ, entityUUID{}, machineUUIDInputs)
	if err != nil {
		return nil, errors.Errorf(
			"preparing select machines filesystem uuids statement: %w", err,
		)
	}

	var dbFilesystemUUIDs []entityUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machinesExist, err := checkMachinesExist(ctx, st, tx, machineUUIDInputs)
		if err != nil {
			return err
		}
		if !machinesExist {
			return errors.New(
				"one or more supplied machines does not exist in the model",
			).Add(domainmachineerrors.MachineNotFound)
		}

		err = tx.Query(ctx, stmt, machineUUIDInputs).GetAll(&dbFilesystemUUIDs)
		if errors.Is(err, sqlair.ErrNoRows) {
			// no rows is not an error and just means that no filesystems are
			// attached to any of the machines.
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Errorf(
			"getting filesystem uuids attached to supplied machines: %w", err,
		)
	}

	retVal := make([]domainstorage.FilesystemUUID, 0, len(dbFilesystemUUIDs))
	for _, dbFilesystemUUID := range dbFilesystemUUIDs {
		retVal = append(retVal, domainstorage.FilesystemUUID(dbFilesystemUUID.UUID))
	}

	return retVal, nil
}
