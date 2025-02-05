// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/internal/errors"
)

// insertStorage constructs inserts storage directive records for the application.
func (st *State) insertStorage(ctx context.Context, tx *sqlair.TX, appDetails applicationDetails, appStorage []application.AddApplicationStorageArg) error {
	if len(appStorage) == 0 {
		return nil
	}

	// This check is here until we rework all of the AddApplication logic to
	// run in a single transaction. There's a TO-DO in the AddApplication service method.
	queryStmt, err := st.Prepare(`
SELECT &charmStorage.name FROM charm_storage
WHERE charm_uuid = $applicationDetails.charm_uuid
`, appDetails, charmStorage{})
	if err != nil {
		return errors.Capture(err)
	}

	var storageMetadata []charmStorage
	err = tx.Query(ctx, queryStmt, appDetails).GetAll(&storageMetadata)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return errors.Errorf("querying supported charm storage: %w", err)
	}
	supportedStorage := set.NewStrings()
	for _, stor := range storageMetadata {
		supportedStorage.Add(stor.Name)
	}
	wantStorage := set.NewStrings()
	for _, stor := range appStorage {
		wantStorage.Add(stor.Name)
	}
	unsupportedStorage := wantStorage.Difference(supportedStorage)
	if unsupportedStorage.Size() > 0 {
		return errors.Errorf("storage %q is not supported", unsupportedStorage.SortedValues())
	}

	storage := make([]storageToAdd, len(appStorage))
	for i, stor := range appStorage {
		storage[i] = storageToAdd{
			ApplicationUUID: appDetails.UUID.String(),
			CharmUUID:       appDetails.CharmID,
			StorageName:     stor.Name,
			StoragePool:     stor.Pool,
			Size:            uint(stor.Size),
			Count:           uint(stor.Count),
		}
	}

	insertStmt, err := st.Prepare(`
INSERT INTO application_storage_directive (*)
VALUES ($storageToAdd.*)`, storageToAdd{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertStmt, storage).Run()
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}
