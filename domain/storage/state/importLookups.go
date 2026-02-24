// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/internal/errors"
)

// storageInstanceLookups contains type lookup tables with values from
// db used in bulk inserts as part of model migration of storage.
type storageInstanceLookups struct {
	Kind            map[string]int
	StoragePoolUUID map[string]string
}

func (st *State) getImportStorageInstanceLookups(ctx context.Context, tx *sqlair.TX) (storageInstanceLookups, error) {
	kindIDs, err := st.getLookupForStorageKind(ctx, tx)
	if err != nil {
		return storageInstanceLookups{}, err
	}
	storagePoolUUIDs, err := st.getStoragePoolUUIDMappings(ctx, tx)
	if err != nil {
		return storageInstanceLookups{}, err
	}
	return storageInstanceLookups{
		Kind:            kindIDs,
		StoragePoolUUID: storagePoolUUIDs,
	}, nil
}

// getLookupForLife retrieves a mapping of kind to id from
// the storage_kind table.
func (st *State) getLookupForStorageKind(ctx context.Context, tx *sqlair.TX) (map[string]int, error) {
	deviceTypeStmt, err := st.Prepare("SELECT &idAndKind.* FROM storage_kind", idAndKind{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var types []idAndKind
	if err := tx.Query(ctx, deviceTypeStmt).GetAll(&types); err != nil {
		return nil, errors.Errorf("querying kinds from storage_kind: %w", err)
	}

	return transform.SliceToMap(types, func(in idAndKind) (string, int) { return in.Kind, in.ID }), nil
}

// getStoragePoolUUIDMappings retrieves a mapping of name to uuid from
// the storage_pool table.
func (st *State) getStoragePoolUUIDMappings(ctx context.Context, tx *sqlair.TX) (map[string]string, error) {
	deviceTypeStmt, err := st.Prepare("SELECT &nameAndUUID.* FROM storage_pool", nameAndUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var types []nameAndUUID
	if err := tx.Query(ctx, deviceTypeStmt).GetAll(&types); err != nil {
		return nil, errors.Errorf("querying storage pool UUIDs: %w", err)
	}

	return transform.SliceToMap(types, func(in nameAndUUID) (string, string) { return in.Name, in.UUID }), nil
}
