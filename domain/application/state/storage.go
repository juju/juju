// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainsequence "github.com/juju/juju/domain/sequence"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// These consts are the sequence namespaces used to generate
// monotonically increasing ints to use for storage entity IDs.
const (
	filesystemNamespace = "filesystem"
	volumeNamespace     = "volume"
	storageNamespace    = "storage"
)

func (st *State) loadStoragePoolUUIDByName(ctx context.Context, tx *sqlair.TX, poolNames []string) (map[string]string, error) {
	type poolnames []string
	storageQuery, err := st.Prepare(`
SELECT &storagePool.*
FROM   storage_pool
WHERE  name IN ($poolnames[:])
`, storagePool{}, poolnames{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var dbPools []storagePool
	err = tx.Query(ctx, storageQuery, poolnames(poolNames)).GetAll(&dbPools)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("querying storage pools: %w", err)
	}
	poolsByName := make(map[string]string)
	for _, p := range dbPools {
		poolsByName[p.Name] = p.UUID
	}
	return poolsByName, nil
}

// insertApplicationStorage constructs inserts storage directive records for the application.
func (st *State) insertApplicationStorage(ctx context.Context, tx *sqlair.TX, appDetails applicationDetails, appStorage []application.ApplicationStorageArg) error {
	if len(appStorage) == 0 {
		return nil
	}

	// This check is here until we rework all of the AddApplication logic to
	// run in a single transaction. There's a TO-DO in the AddApplication service method.
	queryStmt, err := st.Prepare(`
SELECT &charmStorage.name FROM charm_storage
WHERE  charm_uuid = $applicationDetails.charm_uuid
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
		wantStorage.Add(stor.Name.String())
	}
	unsupportedStorage := wantStorage.Difference(supportedStorage)
	if unsupportedStorage.Size() > 0 {
		return errors.Errorf("storage %q is not supported", unsupportedStorage.SortedValues())
	}

	// Storage is either a storage type or a pool name.
	// Get a mapping of pool name to pool UUID for any
	// pools specified in the app storage directives.
	poolNames := make([]string, len(appStorage))
	for i, stor := range appStorage {
		poolNames[i] = stor.PoolNameOrType
	}
	poolsByName, err := st.loadStoragePoolUUIDByName(ctx, tx, poolNames)
	if err != nil {
		return errors.Errorf("loading storage pool UUIDs: %w", err)
	}

	storage := make([]storageToAdd, len(appStorage))
	for i, stor := range appStorage {
		storage[i] = storageToAdd{
			ApplicationUUID: appDetails.UUID.String(),
			CharmUUID:       appDetails.CharmID,
			StorageName:     stor.Name.String(),
			Size:            uint(stor.Size),
			Count:           uint(stor.Count),
		}
		// PoolNameOrType has already been validated to either be
		// a pool name or a valid storage type for the relevant cloud.
		if uuid, ok := poolsByName[stor.PoolNameOrType]; ok {
			storage[i].StoragePoolUUID = &uuid
		} else {
			storage[i].StorageType = &stor.PoolNameOrType
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

type storageTemplate struct {
	meta   charmStorage
	params application.ApplicationStorageArg
}

func (st *State) composeStorageTemplates(ctx context.Context, tx *sqlair.TX, appUUID coreapplication.ID, args []application.ApplicationStorageArg) ([]storageTemplate, error) {
	templates := make([]storageTemplate, 0, len(args))
	for _, arg := range args {
		storageMeta, err := st.getApplicationCharmStorageByName(ctx, tx, appUUID, arg.Name)
		if errors.Is(err, charmStorageNotFound) {
			return nil, errors.Errorf(
				"charm for application %q has no storage called %q",
				appUUID, arg.Name,
			).Add(applicationerrors.StorageNameNotSupported)
		} else if err != nil {
			return nil, errors.Errorf("getting charm storage metadata for storage name %q application %q: %w", arg.Name, appUUID, err)
		}

		if arg.Count == 0 {
			continue
		}
		templates = append(templates, storageTemplate{
			meta:   storageMeta,
			params: arg,
		})
	}
	return templates, nil
}

// insertUnitStorage inserts the storage records need to record the intent
// for the specified new unit and storage args. Records include:
// - storage instance
// - filesystem
// - volume
// - related attachment records
// TODO(storage) - support attaching existing storage when adding a unit
func (st *State) insertUnitStorage(
	ctx context.Context, tx *sqlair.TX,
	appUUID coreapplication.ID,
	unitUUID coreunit.UUID,
	args []application.ApplicationStorageArg, poolKinds map[string]storage.StorageKind,
) ([]attachStorageArgs, error) {

	// Reduce the count of new storage created for each existing storage
	// being attached.
	// TODO(storage) - implement this when unit machine storage can be supported
	// (includes ensureCharmStorageCountChange below)

	templates, err := st.composeStorageTemplates(ctx, tx, appUUID, args)
	if err != nil {
		return nil, errors.Errorf("composing storage info for application %q: %w", appUUID, err)
	}
	if len(templates) == 0 {
		return nil, nil
	}

	result := make([]attachStorageArgs, len(templates))

	app := applicationID{ID: appUUID}
	selectCharmStmt, err := st.Prepare(
		`SELECT &applicationCharmUUID.charm_uuid FROM application WHERE uuid = $applicationID.uuid`,
		app, applicationCharmUUID{})
	if err != nil {
		return result, errors.Capture(err)
	}
	var appCharm applicationCharmUUID
	err = tx.Query(ctx, selectCharmStmt, app).Get(&appCharm)
	if err != nil {
		return result, errors.Errorf("getting application charm for %q: %w", appUUID, err)
	}

	// Storage is either a storage type or a pool name.
	// Get a mapping of pool name to pool UUID for any
	// pools specified in the app storage args.
	poolNames := make([]string, len(args))
	for i, stor := range args {
		poolNames[i] = stor.PoolNameOrType
	}
	poolsByName, err := st.loadStoragePoolUUIDByName(ctx, tx, poolNames)
	if err != nil {
		return result, errors.Errorf("loading storage pool UUIDs: %w", err)
	}

	for i, t := range templates {
		if err := ensureCharmStorageCountChange(t.meta, 0, t.params.Count); err != nil {
			return result, errors.Capture(err)
		}
		result[i].instArgs = make([]storageInstanceArg, t.params.Count)
		for c := range t.params.Count {
			// First create the storage instance records.
			instUUID, err := corestorage.NewUUID()
			if err != nil {
				return result, errors.Capture(err)
			}
			id, err := domainsequence.NextValue(ctx, st, tx, storageNamespace)
			if err != nil {
				return result, errors.Errorf("generating next storage ID: %w", err)
			}
			storageID := corestorage.MakeID(t.params.Name, id)

			result[i].instArgs[c] = storageInstanceArg{
				StorageUUID: instUUID,
				StorageID:   storageID,
			}

			inst := storageInstance{
				StorageUUID:      instUUID,
				StorageID:        storageID,
				StorageName:      t.params.Name,
				RequestedSizeMIB: t.params.Size,
				LifeID:           life.Alive,
				CharmUUID:        appCharm.CharmUUID,
			}
			// PoolNameOrType has already been validated to either be
			// a pool name or a valid storage type for the relevant cloud.
			if uuid, ok := poolsByName[t.params.PoolNameOrType]; ok {
				inst.StoragePoolUUID = &uuid
			} else {
				inst.StorageType = &t.params.PoolNameOrType
			}

			if err := st.createUnitStorageInstance(ctx, tx, unitUUID, inst); err != nil {
				return result, errors.Capture(err)
			}

			// TODO(storage) - insert data for the unit's assigned machine when that is implemented
		}
		result[i].meta = t.meta
		result[i].PoolNameOrType = t.params.PoolNameOrType
	}
	return result, nil
}

type storageInstanceArg struct {
	StorageUUID corestorage.UUID
	StorageID   corestorage.ID
}

type attachStorageArgs struct {
	meta           charmStorage
	PoolNameOrType string
	instArgs       []storageInstanceArg
}

func (st *State) attachUnitStorage(
	ctx context.Context, tx *sqlair.TX,
	storageParentDir string,
	poolKinds map[string]storage.StorageKind,
	unitUUID coreunit.UUID,
	netNodeUUID string,
	args []attachStorageArgs,
) error {

	// Reduce the count of new storage created for each existing storage
	// being attached.
	// TODO(storage) - implement this when unit machine storage can be supported
	// (includes ensureCharmStorageCountChange below)

	for _, arg := range args {
		count := uint64(len(arg.instArgs))
		if err := ensureCharmStorageCountChange(arg.meta, 0, count); err != nil {
			return err
		}
		for _, instArg := range arg.instArgs {
			storageUUID := instArg.StorageUUID
			err := st.attachStorageToUnit(ctx, tx, storageUUID, unitUUID)
			if err != nil {
				return errors.Errorf("attaching storage %q to unit %q: %w", storageUUID, unitUUID, err)
			}

			// Get the info needed to create the necessary filesystem and/or volume attachments to the net node.
			// The required attachments then inform the creation of the filesystem and/or volume.
			filesystem, volume, err := st.attachmentParamsForNewStorageInstance(storageParentDir, instArg.StorageID, arg.PoolNameOrType, arg.meta, poolKinds)
			if err != nil {
				return errors.Errorf("creating storage parameters: %w", err)
			}
			if filesystem != nil {
				filesystemUUID, err := st.createFilesystem(ctx, tx, storageUUID, netNodeUUID)
				if err != nil {
					return errors.Errorf("creating filesystem for storage %q for unit %q: %w", storageUUID, unitUUID, err)
				}
				filesystem.filesystemUUID = filesystemUUID
				if err := st.attachFilesystemToNode(ctx, tx, netNodeUUID, *filesystem); err != nil {
					return errors.Errorf("attaching filesystem to storage %q for unit %q: %w", storageUUID, unitUUID, err)
				}
			}
			if volume != nil {
				volumeUUID, err := st.createVolume(ctx, tx, storageUUID, netNodeUUID)
				if err != nil {
					return errors.Errorf("creating volume for storage %q for unit %q: %w", storageUUID, unitUUID, err)
				}
				volume.volumeUUID = volumeUUID
				if err := st.attachVolumeToNode(ctx, tx, netNodeUUID, *volume); err != nil {
					return errors.Errorf("attaching volume to storage %q for unit %q: %w", storageUUID, unitUUID, err)
				}
			}
		}
	}
	return nil
}

func (st *State) createUnitStorageInstance(ctx context.Context, tx *sqlair.TX, unitUUID coreunit.UUID, inst storageInstance) error {
	insertStorageStmt, err := st.Prepare(`
INSERT INTO storage_instance (*) VALUES ($storageInstance.*)
`, inst)
	if err != nil {
		return errors.Capture(err)
	}

	storageUnit := storageUnit{
		StorageUUID: inst.StorageUUID,
		UnitUUID:    unitUUID,
	}

	insertStorageUnitStmt, err := st.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($storageUnit.*)
	`, storageUnit)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, insertStorageStmt, inst).Run()
	if err != nil {
		return errors.Errorf("creating storage instance %q for unit %q: %w", inst.StorageUUID, unitUUID, err)
	}

	err = tx.Query(ctx, insertStorageUnitStmt, storageUnit).Run()
	if err != nil {
		return errors.Errorf("creating storage unit owner for storage %q and unit %q: %w", inst.StorageUUID, unitUUID, err)
	}
	return nil
}

func (st *State) attachmentParamsForNewStorageInstance(
	parentDir string,
	storageID corestorage.ID,
	poolName string,
	stor charmStorage,
	poolKinds map[string]storage.StorageKind,
) (filesystem *filesystemAttachmentParams, volume *volumeAttachmentParams, _ error) {

	switch charm.StorageType(stor.Kind) {
	case charm.StorageFilesystem:
		location, err := domainstorage.FilesystemMountPoint(parentDir, stor.Location, stor.CountMax, storageID)
		if err != nil {
			return nil, nil, errors.Errorf(
				"getting filesystem mount point for storage %s: %w",
				stor.Name, err,
			).Add(applicationerrors.InvalidStorageMountPoint)
		}
		filesystem = &filesystemAttachmentParams{
			locationAutoGenerated: stor.Location == "", // auto-generated location
			location:              location,
			readOnly:              stor.ReadOnly,
		}
		// For volume backed filesystem storage, we also need to
		// include the backing volume.
		k, ok := poolKinds[poolName]
		if !ok || k == storage.StorageKindFilesystem {
			break
		}
		fallthrough
	case charm.StorageBlock:
		volume = &volumeAttachmentParams{
			readOnly: stor.ReadOnly,
		}
	default:
		return nil, nil, errors.Errorf("invalid storage kind %v", stor.Kind)
	}
	return filesystem, volume, nil
}

// GetStorageUUIDByID returns the UUID for the specified storage, returning an error
// satisfying [storageerrors.StorageNotFound] if the storage doesn't exist.
func (st *State) GetStorageUUIDByID(ctx context.Context, storageID corestorage.ID) (corestorage.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	inst := storageInstance{StorageID: storageID}

	query, err := st.Prepare(`
SELECT &storageInstance.*
FROM   storage_instance
WHERE  storage_id = $storageInstance.storage_id
`, inst)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, inst).Get(&inst)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("storage %q not found", storageID).Add(storageerrors.StorageNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying storage ID %q: %w", storageID, err)
	}

	return inst.StorageUUID, nil
}

// AttachStorage attaches the specified storage to the specified unit.
// The following error types can be expected:
// - [storageerrors.StorageNotFound] when the storage doesn't exist.
// - [applicationerrors.UnitNotFound]: when the unit does not exist.
// - [applicationerrors.StorageAlreadyAttached]: when the attachment already exists.
// - [applicationerrors.FilesystemAlreadyAttached]: when the filesystem is already attached.
// - [applicationerrors.VolumeAlreadyAttached]: when the volume is already attached.
// - [applicationerrors.UnitNotAlive]: when the unit is not alive.
// - [applicationerrors.StorageNotAlive]: when the storage is not alive.
// - [applicationerrors.StorageNameNotSupported]: when storage name is not defined in charm metadata.
// - [applicationerrors.InvalidStorageCount]: when the allowed attachment count would be violated.
// - [applicationerrors.InvalidStorageMountPoint]: when the filesystem being attached to the unit's machine has a mount point path conflict.
func (st *State) AttachStorage(ctx context.Context, storageParentDir string, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	countQuery, err := st.Prepare(`
SELECT count(*) AS &storageCount.count
FROM storage_instance si
JOIN storage_unit_owner suo ON si.uuid = suo.storage_instance_uuid
WHERE suo.unit_uuid = $storageCount.unit_uuid
AND si.storage_name = $storageCount.storage_name
AND si.uuid != $storageCount.uuid
`, storageCount{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// First to the basic life checks for the unit and storage.
		unitLifeID, netNodeUUID, err := st.getUnitLifeAndNetNode(ctx, tx, unitUUID)
		if err != nil {
			return err
		}
		if unitLifeID != life.Alive {
			return errors.Errorf("unit %q is not alive", unitUUID).Add(applicationerrors.UnitNotAlive)
		}

		stor, err := st.getStorageDetails(ctx, tx, storageUUID)
		if err != nil {
			return err
		}
		if stor.LifeID != life.Alive {
			return errors.Errorf("storage %q is not alive", unitUUID).Add(applicationerrors.StorageNotAlive)
		}

		// See if the storage name is supported by the unit's current charm.
		// We might be attaching a storage instance created for a previous charm version
		// and no longer supported.
		charmStorage, err := st.getUnitCharmStorageByName(ctx, tx, unitUUID, stor.StorageName)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"charm for unit %q has no storage called %q",
				unitUUID, stor.StorageName,
			).Add(applicationerrors.StorageNameNotSupported)
		} else if err != nil {
			return errors.Errorf("getting charm storage metadata for storage name %q unit %q: %w", stor.StorageName, unitUUID, err)
		}

		// Check allowed storage attachment counts - will this attachment exceed the max allowed.
		// First get the number of storage instances (excluding the one we are attaching)
		// of the same name already owned by this unit.
		storageCount := storageCount{StorageUUID: storageUUID, StorageName: stor.StorageName, UnitUUID: unitUUID}
		err = tx.Query(ctx, countQuery, storageCount).Get(&storageCount)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying storage count for storage %q on unit %q: %w", stor.StorageName, unitUUID, err)
		}
		// Ensure that the attachment count can increase by 1.
		if err := ensureCharmStorageCountChange(charmStorage, storageCount.Count, 1); err != nil {
			return err
		}

		return st.attachStorage(ctx, tx, stor, unitUUID, netNodeUUID, storageParentDir, charmStorage)
	})
	if err != nil {
		return errors.Errorf("attaching storage %q to unit %q: %w", storageUUID, unitUUID, err)
	}
	return nil
}

func (st *State) AddStorageForUnit(ctx context.Context, storageName corestorage.Name, unitUUID coreunit.UUID, directive storage.Directive) ([]corestorage.ID, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (st *State) DetachStorageForUnit(ctx context.Context, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error {
	//TODO implement me
	return errors.New("not implemented")
}

func (st *State) DetachStorage(ctx context.Context, storageUUID corestorage.UUID) error {
	//TODO implement me
	return errors.New("not implemented")
}

func (st *State) getStorageDetails(ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID) (storageInstance, error) {
	inst := storageInstance{StorageUUID: storageUUID}
	query := `
SELECT &storageInstance.*
FROM   storage_instance
WHERE  uuid = $storageInstance.uuid
`
	queryStmt, err := st.Prepare(query, inst)
	if err != nil {
		return storageInstance{}, errors.Capture(err)
	}

	err = tx.Query(ctx, queryStmt, inst).Get(&inst)
	if err != nil {
		if !errors.Is(err, sqlair.ErrNoRows) {
			return storageInstance{}, errors.Errorf("querying storage %q life: %w", storageUUID, err)
		}
		return storageInstance{}, errors.Errorf("%w: %s", storageerrors.StorageNotFound, storageUUID)
	}
	return inst, nil
}

const charmStorageNotFound = errors.ConstError("charm storage not found")

func (st *State) getApplicationCharmStorageByName(ctx context.Context, tx *sqlair.TX, uuid coreapplication.ID, name corestorage.Name) (charmStorage, error) {
	storageSpec := appCharmStorage{
		ApplicationUUID: uuid,
		StorageName:     name,
	}
	var result charmStorage
	stmt, err := st.Prepare(`
SELECT cs.* AS &charmStorage.*
FROM   v_charm_storage cs
JOIN   application ON application.charm_uuid = cs.charm_uuid
WHERE  application.uuid = $appCharmStorage.uuid
AND    cs.name = $appCharmStorage.name
`, storageSpec, result)
	if err != nil {
		return result, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, storageSpec).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return result, charmStorageNotFound
	}
	if err != nil {
		return result, errors.Errorf("failed to select charm storage: %w", err)
	}

	return result, nil
}

func (st *State) getUnitCharmStorageByName(ctx context.Context, tx *sqlair.TX, uuid coreunit.UUID, name corestorage.Name) (charmStorage, error) {
	storageSpec := unitCharmStorage{
		UnitUUID:    uuid,
		StorageName: name,
	}
	var result charmStorage
	stmt, err := st.Prepare(`
SELECT cs.* AS &charmStorage.*
FROM   v_charm_storage cs
JOIN   unit ON unit.charm_uuid = cs.charm_uuid
WHERE  unit.uuid = $unitCharmStorage.uuid
AND    cs.name = $unitCharmStorage.name
`, storageSpec, result)
	if err != nil {
		return result, errors.Capture(err)
	}

	if err := tx.Query(ctx, stmt, storageSpec).Get(&result); err != nil {
		return result, errors.Errorf("failed to select charm storage: %w", err)
	}

	return result, nil
}

// ensureCharmStorageCountChange checks that the charm storage can change by
// the specified (positive or negative) increment. This is a backstop - the service
// should already have performed the necessary validation.
func ensureCharmStorageCountChange(charmStorage charmStorage, current, n uint64) error {
	action := "attach"
	absn := n
	if n < 0 {
		action = "detach"
		absn = -absn
	}
	gerund := action + "ing"
	pluralise := ""
	if absn != 1 {
		pluralise = "s"
	}

	count := current + n
	if charmStorage.CountMin == 1 && charmStorage.CountMax == 1 && count != 1 {
		return errors.Errorf("cannot %s, storage is singular", action)
	}
	if count < uint64(charmStorage.CountMin) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"which is less than the minimum of %d",
			gerund, absn, pluralise, count,
			charmStorage.CountMin,
		).Add(applicationerrors.InvalidStorageCount)
	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"exceeding the maximum of %d",
			gerund, absn, pluralise, count,
			charmStorage.CountMax,
		).Add(applicationerrors.InvalidStorageCount)
	}
	return nil
}

func (st *State) attachStorage(
	ctx context.Context, tx *sqlair.TX, inst storageInstance, unitUUID coreunit.UUID, netNodeUUID string,
	parentDir string, charmStorage charmStorage,
) error {
	su := storageUnit{StorageUUID: inst.StorageUUID, UnitUUID: unitUUID}
	updateStorageInstanceQuery, err := st.Prepare(`
UPDATE storage_instance
SET    charm_uuid = (SELECT charm_uuid FROM unit where uuid = $storageUnit.unit_uuid)
WHERE  uuid = $storageUnit.storage_instance_uuid
`, su)
	if err != nil {
		return errors.Capture(err)
	}

	err = st.attachStorageToUnit(ctx, tx, inst.StorageUUID, unitUUID)
	if err != nil {
		return errors.Errorf("attaching storage %q to unit %q: %w", inst.StorageUUID, unitUUID, err)
	}

	// TODO(storage) - insert data for the unit's assigned machine when that is implemented

	// Attach volumes and filesystems for reattached storage on CAAS.
	// This only occurs in corner cases where a new pod appears with storage
	// that needs to be reconciled with the Juju model. It is part of the
	// UnitIntroduction workflow when a pod appears with volumes already attached.
	// TODO - this can be removed when ObservedAttachedVolumeIDs are processed.
	modelType, err := st.GetModelType(ctx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}
	if modelType == model.CAAS {
		filesystem, volume, err := st.attachmentParamsForStorageInstance(ctx, tx, parentDir, inst.StorageUUID, inst.StorageID, inst.StorageName, charmStorage)
		if err != nil {
			return errors.Errorf("creating storage attachment parameters: %w", err)
		}
		if filesystem != nil {
			if err := st.attachFilesystemToNode(ctx, tx, netNodeUUID, *filesystem); err != nil {
				return errors.Errorf("attaching filesystem %q to unit %q: %w", filesystem.filesystemUUID, unitUUID, err)
			}
		}
		if volume != nil {
			if err := st.attachVolumeToNode(ctx, tx, netNodeUUID, *volume); err != nil {
				return errors.Errorf("attaching volume %q to unit %q: %w", volume.volumeUUID, unitUUID, err)
			}
		}
	}

	// Update the charm of the storage instance to match the unit to which it is being attached.
	err = tx.Query(ctx, updateStorageInstanceQuery, su).Run()
	if err != nil {
		return errors.Errorf("updating storage instance %q charm: %w", inst.StorageUUID, err)
	}
	return nil
}

type volumeAttachmentParams struct {
	volumeUUID corestorage.VolumeUUID
	readOnly   bool
}

type filesystemAttachmentParams struct {
	// locationAutoGenerated records whether or not the Location
	// field's value was automatically generated, and thus known
	// to be unique. This is used to optimise away mount point
	// conflict checks.
	locationAutoGenerated bool
	filesystemUUID        corestorage.FilesystemUUID
	location              string
	readOnly              bool
}

// getStorageFilesystem gets the filesystem a storage instance is associated with
// and the net node (if any) it is attached to.
func (st *State) getStorageFilesystem(ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID) (filesystemUUID, error) {
	inst := storageInstance{StorageUUID: storageUUID}
	result := filesystemUUID{}
	query, err := st.Prepare(`
SELECT    sf.uuid AS &filesystemUUID.uuid,
          sfa.net_node_uuid AS &filesystemUUID.net_node_uuid
FROM      storage_filesystem sf
JOIN      storage_instance_filesystem sif ON sif.storage_filesystem_uuid = sf.uuid
LEFT JOIN storage_filesystem_attachment sfa ON sfa.storage_filesystem_uuid = sf.uuid
WHERE     sif.storage_instance_uuid = $storageInstance.uuid
	`, inst, result)
	if err != nil {
		return result, errors.Capture(err)
	}
	err = tx.Query(ctx, query, inst).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return result, errors.Errorf("storage filesystem for %q not found", storageUUID).Add(storageerrors.FilesystemNotFound)
	}
	if err != nil {
		return result, errors.Errorf("querying filesystem storage for %q: %w", storageUUID, err)
	}
	return result, nil
}

// getStorageVolume gets the volume a storage instance is associated with
// and the net node (if any) it is attached to.
func (st *State) getStorageVolume(ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID) (volumeUUID, error) {
	inst := storageInstance{StorageUUID: storageUUID}
	result := volumeUUID{}
	query, err := st.Prepare(`
SELECT    sv.uuid AS &volumeUUID.uuid,
          sva.net_node_uuid AS &volumeUUID.net_node_uuid
FROM      storage_volume sv
JOIN      storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
LEFT JOIN storage_volume_attachment sva ON sva.storage_volume_uuid = sv.uuid
WHERE     siv.storage_instance_uuid = $storageInstance.uuid
	`, inst, result)
	if err != nil {
		return result, errors.Capture(err)
	}
	err = tx.Query(ctx, query, inst).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return result, errors.Errorf("storage volume for %q not found", storageUUID).Add(storageerrors.VolumeNotFound)
	}
	if err != nil {
		return result, errors.Errorf("querying volume storage for %q: %w", storageUUID, err)
	}
	return result, nil
}

// attachmentParamsForStorageInstance returns parameters for creating
// volume and filesystem attachments for the specified storage.
func (st *State) attachmentParamsForStorageInstance(
	ctx context.Context,
	tx *sqlair.TX,
	parentDir string,
	storageUUID corestorage.UUID,
	storageID corestorage.ID,
	storageName corestorage.Name,
	charmStorage charmStorage,
) (filesystemResult *filesystemAttachmentParams, volumeResult *volumeAttachmentParams, _ error) {

	switch charm.StorageType(charmStorage.Kind) {
	case charm.StorageFilesystem:
		location, err := domainstorage.FilesystemMountPoint(parentDir, charmStorage.Location, charmStorage.CountMax, storageID)
		if err != nil {
			return nil, nil, errors.Errorf(
				"getting filesystem mount point for storage %s: %w",
				storageName, err,
			).Add(applicationerrors.InvalidStorageMountPoint)
		}

		filesystem, err := st.getStorageFilesystem(ctx, tx, storageUUID)
		if err != nil {
			return nil, nil, errors.Errorf("getting filesystem UUID for storage %q: %w", storageID, err)
		}
		// The filesystem already exists, so just attach it.
		// When creating ops to attach the storage to the
		// machine, we will check if the attachment already
		// exists, and whether the storage can be attached to
		// the machine.
		if !charmStorage.Shared {
			// The storage is not shared, so make sure that it is
			// not currently attached to any other host. If it
			// is, it should be in the process of being detached.
			if filesystem.AttachedTo != nil {
				return nil, nil, errors.Errorf(
					"filesystem %q is attached to %q", filesystem.UUID, *filesystem.AttachedTo).
					Add(applicationerrors.FilesystemAlreadyAttached)
			}
		}
		filesystemResult = &filesystemAttachmentParams{
			locationAutoGenerated: charmStorage.Location == "", // auto-generated location
			location:              location,
			readOnly:              charmStorage.ReadOnly,
			filesystemUUID:        filesystem.UUID,
		}

		// Fall through to attach the volume that backs the filesystem (if any).
		fallthrough

	case charm.StorageBlock:
		volume, err := st.getStorageVolume(ctx, tx, storageUUID)
		if errors.Is(err, storageerrors.VolumeNotFound) && charm.StorageType(charmStorage.Kind) == charm.StorageFilesystem {
			break
		}
		if err != nil {
			return nil, nil, errors.Errorf("getting volume UUID for storage %q: %w", storageID, err)
		}

		// The volume already exists, so just attach it. When
		// creating ops to attach the storage to the machine,
		// we will check if the attachment already exists, and
		// whether the storage can be attached to the machine.
		if !charmStorage.Shared {
			// The storage is not shared, so make sure that it is
			// not currently attached to any other machine. If it
			// is, it should be in the process of being detached.
			if volume.AttachedTo != nil {
				return nil, nil, errors.Errorf("volume %q is attached to %q", volume.UUID, *volume.AttachedTo).
					Add(applicationerrors.VolumeAlreadyAttached)
			}
		}
		volumeResult = &volumeAttachmentParams{
			readOnly:   charmStorage.ReadOnly,
			volumeUUID: volume.UUID,
		}
	default:
		return nil, nil, errors.Errorf("invalid storage kind %v", charmStorage.Kind)
	}
	return filesystemResult, volumeResult, nil
}

func (st *State) attachStorageToUnit(
	ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID, unitUUID coreunit.UUID,
) error {
	sa := storageAttachment{StorageUUID: storageUUID, UnitUUID: unitUUID, LifeID: life.Alive}
	attachmentQuery, err := st.Prepare(`
SELECT &storageAttachment.* FROM storage_attachment
WHERE  unit_uuid = $storageAttachment.unit_uuid
AND    storage_instance_uuid = $storageAttachment.storage_instance_uuid
`, sa)
	if err != nil {
		return errors.Capture(err)
	}

	// See if there's already a row in the storage attachment table.
	var attachments []storageAttachment
	err = tx.Query(ctx, attachmentQuery, sa).GetAll(&attachments)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying storage attachment for storage %q and unit %q: %w", storageUUID, unitUUID, err)
	}
	if err == nil {
		return errors.Errorf("storage %q is already attached to unit %q", storageUUID, unitUUID).Add(applicationerrors.StorageAlreadyAttached)
	}

	stmt, err := st.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($storageAttachment.*)
`, sa)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, sa).Run()
	if err != nil {
		return errors.Errorf("creating unit storage attachment for %q on %q: %w", storageUUID, unitUUID, err)
	}
	return nil
}

func (st *State) attachFilesystemToNode(
	ctx context.Context, tx *sqlair.TX, netNodeUUID string, args filesystemAttachmentParams,
) error {
	uuid, err := corestorage.NewFilesystemAttachmentUUID()
	if err != nil {
		return errors.Capture(err)
	}
	fsa := filesystemAttachment{
		UUID:                 uuid,
		NetNodeUUID:          netNodeUUID,
		FilesystemUUID:       args.filesystemUUID,
		LifeID:               life.Alive,
		MountPoint:           args.location,
		ReadOnly:             args.readOnly,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}
	stmt, err := st.Prepare(`
INSERT INTO storage_filesystem_attachment (*) VALUES ($filesystemAttachment.*)
`, fsa)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, fsa).Run()
	if err != nil {
		return errors.Errorf("creating filesystem attachment for %q on %q: %w", args.filesystemUUID, netNodeUUID, err)
	}
	return nil
}

func (st *State) attachVolumeToNode(
	ctx context.Context, tx *sqlair.TX, netNodeUUID string, args volumeAttachmentParams,
) error {
	uuid, err := corestorage.NewVolumeAttachmentUUID()
	if err != nil {
		return errors.Capture(err)
	}
	fsa := volumeAttachment{
		UUID:                 uuid,
		NetNodeUUID:          netNodeUUID,
		VolumeUUID:           args.volumeUUID,
		LifeID:               life.Alive,
		ReadOnly:             args.readOnly,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}
	stmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment (*) VALUES ($volumeAttachment.*)
`, fsa)
	if err != nil {
		return errors.Capture(err)
	}
	err = tx.Query(ctx, stmt, fsa).Run()
	if err != nil {
		return errors.Errorf("creating volume attachment for %q on %q: %w", args.volumeUUID, netNodeUUID, err)
	}
	return nil
}

func (st *State) createFilesystem(
	ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID, netNodeUUID string,
) (corestorage.FilesystemUUID, error) {
	filesystemId, err := domainsequence.NextValue(ctx, st, tx, filesystemNamespace)
	if err != nil {
		return "", errors.Capture(err)
	}

	uuid, err := corestorage.NewFilesystemUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	fs := filesystem{
		UUID:                 uuid,
		FilesystemID:         fmt.Sprint(filesystemId),
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}
	insertFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provisioning_status_id) VALUES ($filesystem.*)
`, fs)
	if err != nil {
		return "", errors.Capture(err)
	}

	sif := storageInstanceFilesystem{
		FilesystemUUID: uuid,
		StorageUUID:    storageUUID,
	}
	insertStorageFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_instance_filesystem (*) VALUES ($storageInstanceFilesystem.*)
`, sif)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, insertFilesystemStmt, fs).Run()
	if err != nil {
		return "", errors.Errorf("creating filesystem %q for node %q: %w", uuid, netNodeUUID, err)
	}

	err = tx.Query(ctx, insertStorageFilesystemStmt, sif).Run()
	if err != nil {
		return "", errors.Errorf("creating storage instance filesystem %q for storage %q: %w", uuid, storageUUID, err)
	}

	return uuid, nil
}

func (st *State) createVolume(
	ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID, netNodeUUID string,
) (corestorage.VolumeUUID, error) {
	volumeId, err := domainsequence.NextValue(ctx, st, tx, volumeNamespace)
	if err != nil {
		return "", errors.Capture(err)
	}
	volumeUUID, err := corestorage.NewVolumeUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	vol := volume{
		UUID:                 volumeUUID,
		VolumeID:             fmt.Sprint(volumeId),
		LifeID:               life.Alive,
		ProvisioningStatusID: domainstorage.ProvisioningStatusPending,
	}
	insertVolumeStmt, err := st.Prepare(`
INSERT INTO storage_volume (uuid, volume_id, life_id, provisioning_status_id) VALUES ($volume.*)
`, vol)
	if err != nil {
		return "", errors.Errorf("creating storage volume: %w", err)
	}

	siv := storageInstanceVolume{
		VolumeUUID:  volumeUUID,
		StorageUUID: storageUUID,
	}
	insertStorageVolumeStmt, err := st.Prepare(`
INSERT INTO storage_instance_volume (*) VALUES ($storageInstanceVolume.*)
`, siv)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = tx.Query(ctx, insertVolumeStmt, vol).Run()
	if err != nil {
		return "", errors.Errorf("creating volume %q for node %q: %w", volumeUUID, netNodeUUID, err)
	}

	err = tx.Query(ctx, insertStorageVolumeStmt, siv).Run()
	if err != nil {
		return "", errors.Errorf("creating storage instance volume %q for storage %q: %w", volumeUUID, storageUUID, err)
	}
	return volumeUUID, nil
}
