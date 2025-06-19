// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/model"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
	domainsequence "github.com/juju/juju/domain/sequence"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/storage"
)

// These consts are the sequence namespaces used to generate
// monotonically increasing ints to use for storage entity IDs.
const (
	filesystemNamespace = domainsequence.StaticNamespace("filesystem")
	volumeNamespace     = domainsequence.StaticNamespace("volume")
	storageNamespace    = domainsequence.StaticNamespace("storage")
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

// insertApplicationStorageDirectives inserts all of the storage directives for
// a new application. This func checks to make sure that the caller has supplied
// a directive for each of the storage definitions on the charm.
func (st *State) insertApplicationStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	uuid coreapplication.ID,
	charmUUID corecharm.ID,
	directives []application.ApplicationStorageDirectiveArg,
) error {
	if len(directives) == 0 {
		return nil
	}

	insertDirectivesInput := make([]insertApplicationStorageDirective, 0, len(directives))
	for _, d := range directives {
		var (
			poolUUIDVal     sql.Null[string]
			providerTypeVal sql.Null[string]
		)
		if d.PoolUUID != nil {
			poolUUIDVal = sql.Null[string]{
				V:     d.PoolUUID.String(),
				Valid: true,
			}
		}
		if d.ProviderType != nil {
			providerTypeVal = sql.Null[string]{
				V:     *d.ProviderType,
				Valid: true,
			}
		}

		insertDirectivesInput = append(
			insertDirectivesInput,
			insertApplicationStorageDirective{
				ApplicationUUID:     uuid.String(),
				CharmUUID:           charmUUID.String(),
				Count:               d.Count,
				Size:                d.Size,
				StorageName:         d.Name.String(),
				StoragePoolUUID:     poolUUIDVal,
				StorageProviderType: providerTypeVal,
			},
		)
	}

	insertDirectivesStmt, err := st.Prepare(`
INSERT INTO application_storage_directive (*)
VALUES ($insertApplicationStorageDirective.*)
`,
		insertApplicationStorageDirective{})
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, insertDirectivesStmt, insertDirectivesInput).Run()
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// unitStorageDirective represents a single storage directive for a unit.
type unitStorageDirective struct {
	CharmUUID       corecharm.ID
	Count           uint32
	Name            string
	Size            uint64
	StoragePoolUUID *string
	StorageProvider *string
	UnitUUID        coreunit.UUID
}

// createUnitStorageDirectives is responisble for creating the storage
// directives for a unit. This func assumes that no storage directives exist
// already for the unit.
//
// The storage directives supply must match the storage defined by the charm.
// It is expected that the caller is satsified this check has been performed.
func (st *State) createUnitStorageDirectives(
	ctx context.Context,
	tx *sqlair.TX,
	unitUUID coreunit.UUID,
	charmUUID corecharm.ID,
	args []application.UnitStorageDirectiveArg,
) ([]unitStorageDirective, error) {
	if len(args) == 0 {
		return []unitStorageDirective{}, nil
	}

	insertStorageDirectiveStmt, err := st.Prepare(`
INSERT INTO unit_storage_directive (*) VALUES ($insertUnitStorageDirective.*)
`,
		insertUnitStorageDirective{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	insertArgs := make([]insertUnitStorageDirective, 0, len(args))
	rval := make([]unitStorageDirective, 0, len(args))
	for _, arg := range args {
		storagePoolUUIDVal := sql.Null[string]{}
		storageTypeVal := sql.Null[string]{}
		if arg.PoolUUID != nil {
			storagePoolUUIDVal = sql.Null[string]{
				V:     arg.PoolUUID.String(),
				Valid: true,
			}
		}
		if arg.ProviderType != nil {
			storageTypeVal = sql.Null[string]{
				V:     *arg.ProviderType,
				Valid: true,
			}
		}
		insertArgs = append(insertArgs, insertUnitStorageDirective{
			CharmUUID:       charmUUID.String(),
			Count:           arg.Count,
			Size:            arg.Size,
			StorageName:     arg.Name.String(),
			StoragePoolUUID: storagePoolUUIDVal,
			StorageType:     storageTypeVal,
			UnitUUID:        unitUUID.String(),
		})

		var (
			poolUUID *string
			provider *string
		)
		if arg.PoolUUID != nil {
			poolUUIDStr := arg.PoolUUID.String()
			poolUUID = &poolUUIDStr
		}
		if arg.ProviderType != nil {
			poolTypeStr := *arg.ProviderType
			provider = &poolTypeStr
		}
		rval = append(rval, unitStorageDirective{
			CharmUUID:       charmUUID,
			Count:           arg.Count,
			Name:            arg.Name.String(),
			StoragePoolUUID: poolUUID,
			StorageProvider: provider,
			Size:            arg.Size,
			UnitUUID:        unitUUID,
		})
	}

	err = tx.Query(ctx, insertStorageDirectiveStmt, insertArgs).Run()
	if err != nil {
		return nil, errors.Errorf("creating unit %q storage directives: %w", unitUUID, err)
	}

	return rval, nil
}

// createUnitStorageInstances is responsible for creating all of the needed
// storage instances to satisfy the set of unit storage directives supplied.
// For every storage instance created, a storage unit owner record is also
// created.
//
// This func assumes that for each unit in the storage directive no storage
// instances have previously been created for this unit and directive.
func (s *State) createUnitStorageInstances(
	ctx context.Context,
	tx *sqlair.TX,
	stDirectives []unitStorageDirective,
) error {
	insertStorageAttachmentArgs := make([]insertStorageAttachment, 0, len(stDirectives))
	insertStorageInstArgs := make([]insertStorageInstance, 0, len(stDirectives))
	insertStorageOwnerArgs := make([]insertStorageUnitOwner, 0, len(stDirectives))
	for _, directive := range stDirectives {
		storageAttachmentArgs, storageInstanceArgs, storageUnitOwnerArgs, err :=
			s.makeInsertUnitStorageArgs(
				ctx, tx, directive,
			)
		if err != nil {
			return errors.Errorf(
				"making storage instance(s) args from unit %q directive %q: %w",
				directive.UnitUUID, directive.Name, err,
			)
		}

		insertStorageAttachmentArgs = append(
			insertStorageAttachmentArgs, storageAttachmentArgs...,
		)
		insertStorageInstArgs = append(
			insertStorageInstArgs, storageInstanceArgs...,
		)
		insertStorageOwnerArgs = append(
			insertStorageOwnerArgs, storageUnitOwnerArgs...,
		)
	}

	insertStorageAttachmentStmt, err := s.Prepare(`
INSERT INTO storage_attachment (*) VALUES ($insertStorageAttachment.*)
`,
		insertStorageAttachment{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageInstStmt, err := s.Prepare(`
INSERT INTO storage_instance (*) VALUES ($insertStorageInstance.*)
`,
		insertStorageInstance{})
	if err != nil {
		return errors.Capture(err)
	}

	insertStorageOwnerStmt, err := s.Prepare(`
INSERT INTO storage_unit_owner (*) VALUES ($insertStorageUnitOwner.*)
`,
		insertStorageUnitOwner{})
	if err != nil {
		return errors.Capture(err)
	}

	// We gaurd against zero length insert args below. This is because there is
	// no direct correlation to number of storage directives and records
	// created. Empty inserts will result in an error that we don't care about.
	if len(insertStorageInstArgs) != 0 {
		err := tx.Query(ctx, insertStorageInstStmt, insertStorageInstArgs).Run()
		if err != nil {
			return errors.Errorf(
				"creating %d storage instance(s): %w",
				len(insertStorageInstArgs), err,
			)
		}
	}

	if len(insertStorageAttachmentArgs) != 0 {
		err := tx.Query(
			ctx, insertStorageAttachmentStmt, insertStorageAttachmentArgs,
		).Run()
		if err != nil {
			return errors.Errorf(
				"creating %d storage instance attachment(s): %w",
				len(insertStorageAttachmentArgs), err,
			)
		}
	}

	if len(insertStorageOwnerArgs) != 0 {
		err := tx.Query(ctx, insertStorageOwnerStmt, insertStorageOwnerArgs).Run()
		if err != nil {
			return errors.Errorf(
				"setting storage instance unit owner: %w", err,
			)
		}
	}

	return nil
}

// GetProviderTypeOfPool returns the provider type that is in use for the
// given pool.
//
// The following error types can be expected:
// - [storageerrors.PoolNotFoundError] when no storage pool exists for the
// provided pool uuid.
func (st *State) GetProviderTypeOfPool(
	ctx context.Context, poolUUID domainstorage.StoragePoolUUID,
) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		poolUUIDInput = storagePoolUUID{UUID: poolUUID.String()}
		typeVal       storagePoolType
	)

	providerTypeStmt, err := st.Prepare(`
SELECT &storagePoolType.*
FROM   storage_pool
WHERE  uuid = $storagePoolUUID.uuid
`,
		poolUUIDInput, typeVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, providerTypeStmt, poolUUIDInput).Get(&typeVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage pool %q does not exist", poolUUID,
			).Add(storageerrors.PoolNotFoundError)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return typeVal.Type, nil
}

// makeInsertUnitStorageArgs is responsible for making the insert args required
// for instantiating new storage instances that match a unit's storage
// directive Included in the return is the set of insert values required for
// making the unit the owner of the new storage instance(s). Attachment records
// are also returned for each of the storage instances.
func (st *State) makeInsertUnitStorageArgs(
	ctx context.Context,
	tx *sqlair.TX,
	directive unitStorageDirective,
) ([]insertStorageAttachment, []insertStorageInstance, []insertStorageUnitOwner, error) {
	storageAttachmentRval := make([]insertStorageAttachment, 0, directive.Count)
	storageInstanceRval := make([]insertStorageInstance, 0, directive.Count)
	storageOwnerRval := make([]insertStorageUnitOwner, 0, directive.Count)
	for range directive.Count {
		uuid, err := corestorage.NewUUID()
		if err != nil {
			return nil, nil, nil, errors.Errorf(
				"creating storage uuid for new storage instance: %w", err,
			)
		}

		id, err := sequencestate.NextValue(ctx, st, tx, storageNamespace)
		if err != nil {
			return nil, nil, nil, errors.Errorf(
				"creating unique storage instance id: %w", err,
			)
		}

		storageID := corestorage.MakeID(
			corestorage.Name(directive.Name), id,
		).String()

		storagePoolVal := sql.Null[string]{}
		if directive.StoragePoolUUID != nil {
			storagePoolVal.V = *directive.StoragePoolUUID
			storagePoolVal.Valid = true
		}
		storageTypeVal := sql.Null[string]{}
		if directive.StorageProvider != nil {
			storageTypeVal.V = *directive.StorageProvider
			storageTypeVal.Valid = true
		}

		storageAttachmentRval = append(storageAttachmentRval, insertStorageAttachment{
			StorageInstanceUUID: uuid.String(),
			LifeID:              int(life.Alive),
			UnitUUID:            directive.UnitUUID.String(),
		})
		storageInstanceRval = append(storageInstanceRval, insertStorageInstance{
			CharmUUID:       directive.CharmUUID.String(),
			LifeID:          int(life.Alive),
			RequestSizeMiB:  directive.Size,
			StorageID:       storageID,
			StorageName:     directive.Name,
			StoragePoolUUID: storagePoolVal,
			StorageType:     storageTypeVal,
			UUID:            uuid.String(),
		})
		storageOwnerRval = append(storageOwnerRval, insertStorageUnitOwner{
			StorageInstanceUUID: uuid.String(),
			UnitUUID:            directive.UnitUUID.String(),
		})
	}

	return storageAttachmentRval, storageInstanceRval, storageOwnerRval, nil
}

// getApplicationStorageDirectiveAsArgs returns the current set of storage
// directives set for an application as the directive arguments that would have
// been used to create them. This func does not check to make sure that the
// application exists. No error is returned when no storage directives exist.
func (st *State) getApplicationStorageDirectiveAsArgs(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID coreapplication.ID,
) ([]application.ApplicationStorageDirectiveArg, error) {
	appUUIDInput := applicationID{ID: appUUID}

	getStorageDirectivesStmt, err := st.Prepare(`
SELECT &applicationStorageDirective.*
FROM   application_storage_directive
WHERE  application_uuid = $applicationID.uuid
`,
		applicationStorageDirective{}, appUUIDInput)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []applicationStorageDirective
	err = tx.Query(ctx, getStorageDirectivesStmt, appUUIDInput).GetAll(&dbVals)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	rval := make([]application.ApplicationStorageDirectiveArg, 0, len(dbVals))
	for _, val := range dbVals {
		arg := application.ApplicationStorageDirectiveArg{
			Count: val.Count,
			Name:  domainstorage.Name(val.StorageName),
			Size:  val.SizeMiB,
		}
		if val.StoragePoolUUID.Valid {
			poolUUIID := domainstorage.StoragePoolUUID(val.StoragePoolUUID.V)
			arg.PoolUUID = &poolUUIID
		}
		if val.StorageType.Valid {
			providerType := val.StorageType.V
			arg.ProviderType = &providerType
		}

		rval = append(rval, arg)
	}
	return rval, nil
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
func (st *State) AttachStorage(ctx context.Context, storageUUID corestorage.UUID, unitUUID coreunit.UUID) error {
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

		return st.attachStorage(ctx, tx, stor, unitUUID, netNodeUUID, charmStorage)
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

// GetDefaultStorageProvisioners returns the default storage provisioners
// that have been set for the model.
func (st *State) GetDefaultStorageProvisioners(
	ctx context.Context,
) (application.DefaultStorageProvisioners, error) {
	// TODO (tlm) get the default storage provisioners for the model.
	defaultProviderType := "loop"
	return application.DefaultStorageProvisioners{
		BlockdeviceProviderType: &defaultProviderType,
		FilesystemProviderType:  &defaultProviderType,
	}, nil
}

// ensureCharmStorageCountChange checks that the charm storage can change by
// the specified (positive or negative) increment. This is a backstop - the service
// should already have performed the necessary validation.
func ensureCharmStorageCountChange(charmStorage charmStorage, current, n uint64) error {
	action := "attach"
	gerund := action + "ing"
	pluralise := ""
	if n != 1 {
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
			gerund, n, pluralise, count,
			charmStorage.CountMin,
		).Add(applicationerrors.InvalidStorageCount)
	}
	if charmStorage.CountMax >= 0 && count > uint64(charmStorage.CountMax) {
		return errors.Errorf(
			"%s %d storage instance%s brings the total to %d, "+
				"exceeding the maximum of %d",
			gerund, n, pluralise, count,
			charmStorage.CountMax,
		).Add(applicationerrors.InvalidStorageCount)
	}
	return nil
}

func (st *State) attachStorage(
	ctx context.Context, tx *sqlair.TX, inst storageInstance, unitUUID coreunit.UUID, netNodeUUID string,
	charmStorage charmStorage,
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
	modelType, err := st.getModelType(ctx, tx)
	if err != nil {
		return errors.Errorf("getting model type: %w", err)
	}
	if modelType == model.CAAS {
		filesystem, volume, err := st.attachmentParamsForStorageInstance(ctx, tx, inst.StorageUUID, inst.StorageID, inst.StorageName, charmStorage)
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
	storageUUID corestorage.UUID,
	storageID corestorage.ID,
	storageName corestorage.Name,
	charmStorage charmStorage,
) (filesystemResult *filesystemAttachmentParams, volumeResult *volumeAttachmentParams, _ error) {

	switch charm.StorageType(charmStorage.Kind) {
	case charm.StorageFilesystem:
		location, err := domainstorage.FilesystemMountPoint(charmStorage.Location, charmStorage.CountMax, storageID)
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
		UUID:           uuid,
		NetNodeUUID:    netNodeUUID,
		FilesystemUUID: args.filesystemUUID,
		LifeID:         life.Alive,
		MountPoint:     args.location,
		ReadOnly:       args.readOnly,
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
		UUID:        uuid,
		NetNodeUUID: netNodeUUID,
		VolumeUUID:  args.volumeUUID,
		LifeID:      life.Alive,
		ReadOnly:    args.readOnly,
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
	filesystemId, err := sequencestate.NextValue(ctx, st, tx, filesystemNamespace)
	if err != nil {
		return "", errors.Capture(err)
	}

	filesystemUUID, err := corestorage.NewFilesystemUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	fs := filesystem{
		UUID:         filesystemUUID,
		FilesystemID: fmt.Sprint(filesystemId),
		LifeID:       life.Alive,
	}
	insertFilesystemStmt, err := st.Prepare(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id) VALUES ($filesystem.*)
`, fs)
	if err != nil {
		return "", errors.Capture(err)
	}

	sif := storageInstanceFilesystem{
		FilesystemUUID: filesystemUUID,
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
		return "", errors.Errorf("creating filesystem %q for node %q: %w", filesystemUUID, netNodeUUID, err)
	}

	err = tx.Query(ctx, insertStorageFilesystemStmt, sif).Run()
	if err != nil {
		return "", errors.Errorf("creating storage instance filesystem %q for storage %q: %w", filesystemUUID, storageUUID, err)
	}

	sts := status.StatusInfo[status.StorageFilesystemStatusType]{
		Status: status.StorageFilesystemStatusTypePending,
		Since:  ptr(st.clock.Now()),
	}
	if err := st.insertFilesystemStatus(ctx, tx, filesystemUUID, sts); err != nil {
		return "", errors.Errorf("inserting status for filesystem %q: %w", filesystemUUID, err)
	}

	return filesystemUUID, nil
}

func (st *State) insertFilesystemStatus(
	ctx context.Context,
	tx *sqlair.TX,
	fsUUID corestorage.FilesystemUUID,
	sts status.StatusInfo[status.StorageFilesystemStatusType],
) error {
	statusID, err := status.EncodeStorageFilesystemStatus(sts.Status)
	if err != nil {
		return errors.Errorf("encoding status: %w", err)
	}
	fsStatus := filesystemStatus{
		FilesystemUUID: fsUUID.String(),
		StatusID:       statusID,
		UpdatedAt:      sts.Since,
	}
	insertStmt, err := st.Prepare(`
INSERT INTO storage_filesystem_status (*) VALUES ($filesystemStatus.*);
`, fsStatus)
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, fsStatus).Run(); err != nil {
		return errors.Errorf("inserting status: %w", err)
	}
	return nil
}

func (st *State) createVolume(
	ctx context.Context, tx *sqlair.TX, storageUUID corestorage.UUID, netNodeUUID string,
) (corestorage.VolumeUUID, error) {
	volumeId, err := sequencestate.NextValue(ctx, st, tx, volumeNamespace)
	if err != nil {
		return "", errors.Capture(err)
	}
	volumeUUID, err := corestorage.NewVolumeUUID()
	if err != nil {
		return "", errors.Capture(err)
	}

	vol := volume{
		UUID:     volumeUUID,
		VolumeID: fmt.Sprint(volumeId),
		LifeID:   life.Alive,
	}
	insertVolumeStmt, err := st.Prepare(`
INSERT INTO storage_volume (uuid, volume_id, life_id) VALUES ($volume.*)
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

	sts := status.StatusInfo[status.StorageVolumeStatusType]{
		Status: status.StorageVolumeStatusTypePending,
		Since:  ptr(st.clock.Now()),
	}
	if err := st.insertVolumeStatus(ctx, tx, volumeUUID, sts); err != nil {
		return "", errors.Errorf("inserting status for volume %q: %w", volumeUUID, err)
	}

	return volumeUUID, nil
}

func (st *State) insertVolumeStatus(
	ctx context.Context,
	tx *sqlair.TX,
	volUUID corestorage.VolumeUUID,
	sts status.StatusInfo[status.StorageVolumeStatusType],
) error {
	statusID, err := status.EncodeStorageVolumeStatus(sts.Status)
	if err != nil {
		return errors.Errorf("encoding status: %w", err)
	}
	volStatus := volumeStatus{
		VolumeUUID: volUUID.String(),
		StatusID:   statusID,
		UpdatedAt:  sts.Since,
	}
	insertStmt, err := st.Prepare(`
INSERT INTO storage_volume_status (*) VALUES ($volumeStatus.*);
`, volStatus)
	if err != nil {
		return errors.Errorf("preparing insert query: %w", err)
	}

	if err := tx.Query(ctx, insertStmt, volStatus).Run(); err != nil {
		return errors.Errorf("inserting status: %w", err)
	}
	return nil
}
