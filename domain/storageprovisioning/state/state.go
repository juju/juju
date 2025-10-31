// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/application"
	coredatabase "github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
}

// NewState creates and returns a new [State] for provisioning storage in the
// model.
func NewState(
	factory coredatabase.TxnRunnerFactory,
) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// checkMachineExists checks if the supplied machine uuid exists within the
// model.
func (st *State) checkMachineExists(
	ctx context.Context, tx *sqlair.TX, uuid coremachine.UUID,
) (bool, error) {
	input := machineUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(
		"SELECT &machineUUID.* FROM machine WHERE uuid = $machineUUID.uuid",
		input,
	)
	if err != nil {
		return false, errors.Errorf("preparing machine exists statement: %w", err)
	}

	err = tx.Query(ctx, stmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}
	return true, nil
}

// CheckMachineIsDead checks to see if a machine is not dead returning
// true when the life of the machine is dead.
//
// The following errors may be returned:
// - [github.com/juju/juju/domain/machine/errors.MachineNotFound] when no
// machine exists for the provided uuid.
func (st *State) CheckMachineIsDead(
	ctx context.Context, uuid coremachine.UUID,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	var (
		input       = machineUUID{UUID: uuid.String()}
		machineLife entityLife
	)
	stmt, err := st.Prepare(
		"SELECT &entityLife.* FROM machine WHERE uuid = $machineUUID.uuid",
		input, machineLife,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&machineLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q does not exist", uuid).Add(
				machineerrors.MachineNotFound,
			)
		}
		return err
	})

	if err != nil {
		return false, errors.Capture(err)
	}

	return domainlife.Life(machineLife.LifeID) == domainlife.Dead, nil
}

// GetMachineNetNodeUUID retrieves the net node uuid associated with provided
// machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] when no machine exists for the provided
// uuid.
func (st *State) GetMachineNetNodeUUID(
	ctx context.Context, uuid coremachine.UUID,
) (domainnetwork.NetNodeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		machineUUIDInput = machineUUID{UUID: uuid.String()}
		dbVal            netNodeUUIDRef
	)
	stmt, err := st.Prepare(
		"SELECT &netNodeUUIDRef.* FROM machine WHERE uuid = $machineUUID.uuid",
		machineUUIDInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q does not exist", uuid).Add(
				machineerrors.MachineNotFound,
			)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return domainnetwork.NetNodeUUID(dbVal.UUID), nil
}

// GetUnitNetNodeUUID retrieves the net node uuid associated with provided unit.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] when no unit exists for the provided
// uuid.
func (st *State) GetUnitNetNodeUUID(
	ctx context.Context, uuid coreunit.UUID,
) (domainnetwork.NetNodeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		unitUUIDInput = unitUUID{UUID: uuid.String()}
		dbVal         netNodeUUIDRef
	)
	stmt, err := st.Prepare(
		"SELECT &netNodeUUIDRef.* FROM unit WHERE uuid = $unitUUID.uuid",
		unitUUIDInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q does not exist", uuid).Add(
				applicationerrors.UnitNotFound,
			)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return domainnetwork.NetNodeUUID(dbVal.UUID), nil
}

func (st *State) NamespaceForWatchMachineCloudInstance() string {
	return "machine_cloud_instance"
}

// GetStorageResourceTagInfoForApplication returns information required to
// build resource tags for storage created for the given application.
//
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound] when no application exists for the
// supplied uuid.
func (st *State) GetStorageResourceTagInfoForApplication(
	ctx context.Context,
	appUUID application.UUID,
	resourceTagModelConfigKey string,
) (storageprovisioning.ApplicationResourceTagInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.ApplicationResourceTagInfo{}, errors.Capture(err)
	}

	type applicationName struct {
		Name string `db:"name"`
	}

	var (
		appNameVal   = applicationName{Name: appUUID.String()}
		appUUIDInput = entityUUID{UUID: appUUID.String()}
	)

	appNameStmt, err := st.Prepare(`
SELECT &applicationName.*
FROM   application
WHERE  uuid = $entityUUID.uuid
`,
		appNameVal, appUUIDInput)
	if err != nil {
		return storageprovisioning.ApplicationResourceTagInfo{}, errors.Capture(err)
	}

	var modelResourceInfo storageprovisioning.ModelResourceTagInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, appNameStmt, appUUIDInput).Get(&appNameVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"application %q does not exist", appUUID,
			).Add(applicationerrors.ApplicationNotFound)
		} else if err != nil {
			return err
		}

		modelResourceInfo, err = st.getStorageResourceTagInfoForModel(
			ctx, tx, resourceTagModelConfigKey,
		)
		return err
	})
	if err != nil {
		return storageprovisioning.ApplicationResourceTagInfo{}, errors.Capture(err)
	}

	return storageprovisioning.ApplicationResourceTagInfo{
		ModelResourceTagInfo: modelResourceInfo,
		ApplicationName:      appNameVal.Name,
	}, nil
}

// GetStorageResourceTagInfoForModel retrieves the model based resource tag
// information for storage entities.
func (st *State) GetStorageResourceTagInfoForModel(
	ctx context.Context,
	resourceTagModelConfigKey string,
) (storageprovisioning.ModelResourceTagInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	var rval storageprovisioning.ModelResourceTagInfo
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		rval, err = st.getStorageResourceTagInfoForModel(
			ctx, tx, resourceTagModelConfigKey,
		)
		return err
	})

	if err != nil {
		return storageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	return rval, nil
}

// getStorageResourceTagInfoForModel retrieves the model based resource tag
// information for storage entities.
func (st *State) getStorageResourceTagInfoForModel(
	ctx context.Context,
	tx *sqlair.TX,
	resourceTagModelConfigKey string,
) (storageprovisioning.ModelResourceTagInfo, error) {
	type modelConfigKey struct {
		Key string `db:"key"`
	}

	var (
		modelConfigKeyInput = modelConfigKey{Key: resourceTagModelConfigKey}
		dbVal               modelResourceTagInfo
	)

	resourceTagStmt, err := st.Prepare(`
SELECT value AS &modelResourceTagInfo.resource_tags
FROM   model_config
WHERE  key = $modelConfigKey.key
`,
		dbVal, modelConfigKeyInput)
	if err != nil {
		return storageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	modelInfoStmt, err := st.Prepare(`
SELECT (uuid, controller_uuid) AS (&modelResourceTagInfo.*)
FROM model
`,
		dbVal)
	if err != nil {
		return storageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	err = tx.Query(ctx, resourceTagStmt, modelConfigKeyInput).Get(&dbVal)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return storageprovisioning.ModelResourceTagInfo{}, errors.Errorf(
			"getting model config value for key %q: %w",
			resourceTagModelConfigKey, err,
		)
	}

	err = tx.Query(ctx, modelInfoStmt).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		// This must never happen, but we return an error that at least signals
		// the problem correctly in case it does.
		return storageprovisioning.ModelResourceTagInfo{}, errors.New(
			"model database has not had its information set",
		)
	} else if err != nil {
		return storageprovisioning.ModelResourceTagInfo{}, errors.Capture(err)
	}

	return storageprovisioning.ModelResourceTagInfo{
		BaseResourceTags: dbVal.ResourceTags,
		ControllerUUID:   dbVal.ControllerUUID,
		ModelUUID:        dbVal.ModelUUID,
	}, nil
}

// checkNetNodeExists checks if the provided net node uuid exists in the
// database during a txn. False is returned when the net node does not exist.
func (st *State) checkNetNodeExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid domainnetwork.NetNodeUUID,
) (bool, error) {
	input := netNodeUUID{UUID: uuid.String()}

	checkStmt, err := st.Prepare(
		"SELECT &netNodeUUID.* FROM net_node WHERE uuid = $netNodeUUID.uuid",
		input,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

func (st *State) checkUnitExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid coreunit.UUID,
) (bool, error) {
	input := unitUUID{UUID: uuid.String()}

	checkStmt, err := st.Prepare(`
SELECT &unitUUID.*
FROM   unit
WHERE  uuid = $unitUUID.uuid`, input)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

func (st *State) checkStorageInstanceUUIDByStorageID(
	ctx context.Context,
	tx *sqlair.TX,
	storageInstanceID string,
) (bool, error) {

	var (
		idInput = storageID{ID: storageInstanceID}
		dbVal   storageInstanceUUID
	)
	uuidQuery, err := st.Prepare(`
SELECT &storageInstanceUUID.*
FROM   storage_instance
WHERE  storage_id = $storageID.storage_id
`,
		idInput, dbVal,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, uuidQuery, idInput).Get(&dbVal)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

func (st *State) checkStorageInstanceExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storage.StorageInstanceUUID,
) (bool, error) {
	input := entityUUID{UUID: uuid.String()}
	checkStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_instance
WHERE  uuid = $entityUUID.uuid`, input)
	if err != nil {
		return false, errors.Capture(err)
	}
	err = tx.Query(ctx, checkStmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// checkApplicationExists checks if the provided application uuid exists in the
// database during a txn. False is returned when the application does not exist.
func (st *State) checkApplicationExists(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID application.UUID,
) (bool, error) {
	input := entityUUID{UUID: appUUID.String()}

	checkStmt, err := st.Prepare(
		"SELECT &entityUUID.* FROM application WHERE uuid = $entityUUID.uuid",
		input,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkStmt, input).Get(&input)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// GetStorageAttachmentIDsForUnit returns the storage attachment IDs for the
// given unit UUID.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] when no unit exists for the supplied unit
// UUID.
func (st *State) GetStorageAttachmentIDsForUnit(
	ctx context.Context, unitUUID coreunit.UUID,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := unitUUIDRef{UUID: unitUUID.String()}
	stmt, err := st.Prepare(`
SELECT &storageID.*
FROM   storage_attachment sa
JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
WHERE  unit_uuid = $unitUUIDRef.unit_uuid`, input, storageID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var ids storageIDs
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, unitUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if !exists {
			return errors.Errorf("unit %q does not exist", unitUUID).Add(
				applicationerrors.UnitNotFound,
			)
		}

		err = tx.Query(ctx, stmt, input).GetAll(&ids)
		if errors.Is(err, sqlair.ErrNoRows) {
			// No storage attachments for the unit, return an empty slice.
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	storageIDs := make([]string, len(ids))
	for i, id := range ids {
		storageIDs[i] = id.ID
	}
	return storageIDs, nil
}

// GetStorageAttachmentLifeForUnit returns a mapping of storage IDs to the
// current life value of each storage attachment for the unit.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
func (st *State) GetStorageAttachmentLifeForUnit(
	ctx context.Context,
	unitUUID coreunit.UUID,
) (map[string]domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getStorageAttachmentLifeForUnit(ctx, db, unitUUID)
}

func (st *State) getStorageAttachmentLifeForUnit(
	ctx context.Context,
	db domain.TxnRunner,
	uuid coreunit.UUID,
) (map[string]domainlife.Life, error) {
	unitUUIDInput := unitUUID{UUID: uuid.String()}

	stmt, err := st.Prepare(`
SELECT (si.storage_id, sa.life_id) AS (&storageAttachmentLife.*)
FROM   storage_attachment sa
JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
WHERE  sa.unit_uuid = $unitUUID.uuid`, unitUUIDInput, storageAttachmentLife{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var saLives storageAttachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if exists, err := st.checkUnitExists(ctx, tx, uuid); err != nil {
			return errors.Capture(err)
		} else if !exists {
			return errors.Errorf("unit %q does not exist", uuid).Add(
				applicationerrors.UnitNotFound,
			)
		}

		err = tx.Query(ctx, stmt, unitUUIDInput).GetAll(&saLives)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(saLives.Iter), nil
}

// GetStorageInstanceUUIDByID retrieves the UUID of a storage instance by its ID.
//
// The following errors may be returned:
// - [storageerrors.StorageInstanceNotFound] when no storage
// instance exists for the provided ID.
func (st *State) GetStorageInstanceUUIDByID(
	ctx context.Context, storageIDStr string,
) (storage.StorageInstanceUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	input := storageID{ID: storageIDStr}
	dbVal := entityUUID{}
	stmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_instance
WHERE  storage_id = $storageID.storage_id`, input, dbVal)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage instance with ID %q does not exist", storageIDStr,
			).Add(storageerrors.StorageInstanceNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}
	return storage.StorageInstanceUUID(dbVal.UUID), nil
}

// GetStorageAttachmentInfo returns information about a storage attachment for
// the given storage attachment UUID.
//
// The following errors may be returned:
// - [storageprovisioningerrors.StorageAttachmentNotFound] when the storage
// attachment does not exist.
func (st *State) GetStorageAttachmentInfo(
	ctx context.Context, uuid storageprovisioning.StorageAttachmentUUID,
) (storageprovisioning.StorageAttachmentInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.StorageAttachmentInfo{}, errors.Capture(err)
	}

	input := storageAttachmentUUID{UUID: uuid.String()}
	var dbVal storageAttachmentInfo
	stmt, err := st.Prepare(`
SELECT &storageAttachmentInfo.* FROM (
    SELECT    sa.life_id,
              si.storage_kind_id,
              sa.uuid AS storage_attachment_uuid,
              sfa.mount_point,
              sva.block_device_uuid
    FROM      storage_attachment sa
    JOIN      storage_instance si ON sa.storage_instance_uuid = si.uuid
    JOIN      unit u ON sa.unit_uuid=u.uuid
    LEFT JOIN storage_instance_volume siv ON si.uuid=siv.storage_instance_uuid
    LEFT JOIN storage_volume_attachment sva ON siv.storage_volume_uuid=sva.storage_volume_uuid AND sva.net_node_uuid=u.net_node_uuid
    LEFT JOIN storage_instance_filesystem sif ON si.uuid=sif.storage_instance_uuid
    LEFT JOIN storage_filesystem_attachment sfa ON sif.storage_filesystem_uuid=sfa.storage_filesystem_uuid AND sfa.net_node_uuid=u.net_node_uuid
    WHERE     sa.uuid = $storageAttachmentUUID.uuid
)`, input, dbVal)
	if err != nil {
		return storageprovisioning.StorageAttachmentInfo{}, errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage attachment with UUID %q does not exist", uuid,
			).Add(storageerrors.StorageAttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return storageprovisioning.StorageAttachmentInfo{}, errors.Capture(err)
	}
	info := storageprovisioning.StorageAttachmentInfo{
		Kind:                 storage.StorageKind(dbVal.KindID),
		Life:                 dbVal.Life,
		FilesystemMountPoint: dbVal.FilesystemMountPoint,
		BlockDeviceUUID:      blockdevice.BlockDeviceUUID(dbVal.BlockDeviceUUID),
	}
	return info, nil
}

// GetStorageAttachmentLife retrieves the life of a storage attachment for a unit
// and the storage instance.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] when no unit exists for the supplied
// unit UUID.
// - [storageerrors.StorageInstanceNotFound] when no storage
// instance exists for the provided storage instance UUID.
// - [storageerrors.StorageAttachmentNotFound] when
// the storage attachment does not exist for the unit and storage instance.
func (st *State) GetStorageAttachmentLife(
	ctx context.Context,
	unitUUID coreunit.UUID,
	storageInstanceUUID storage.StorageInstanceUUID,
) (domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}
	input := storageAttachmentIdentifier{
		StorageInstanceUUID: storageInstanceUUID.String(),
		UnitUUID:            unitUUID.String(),
	}
	attachmentLife := entityLife{}
	stmt, err := st.Prepare(`
SELECT &entityLife.*
FROM   storage_attachment
WHERE  unit_uuid = $storageAttachmentIdentifier.unit_uuid
AND    storage_instance_uuid = $storageAttachmentIdentifier.storage_instance_uuid
`, input, attachmentLife)
	if err != nil {
		return -1, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if exists, err := st.checkUnitExists(ctx, tx, unitUUID); err != nil {
			return errors.Capture(err)
		} else if !exists {
			return errors.Errorf(
				"unit %q does not exist", unitUUID,
			).Add(applicationerrors.UnitNotFound)
		}
		exists, err := st.checkStorageInstanceExists(
			ctx, tx, storageInstanceUUID)
		if err != nil {
			return errors.Capture(err)
		} else if !exists {
			return errors.Errorf(
				"storage instance %q does not exist", storageInstanceUUID,
			).Add(storageerrors.StorageInstanceNotFound)
		}

		err = tx.Query(ctx, stmt, input).Get(&attachmentLife)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage attachment for unit %q and storage instance %q does not exist",
				unitUUID, storageInstanceUUID,
			).Add(storageerrors.StorageAttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return -1, errors.Capture(err)
	}
	return domainlife.Life(attachmentLife.LifeID), nil
}

// InitialWatchStatementForUnitStorageAttachments returns the initial watch
// statement for unit storage attachments.
func (st *State) InitialWatchStatementForUnitStorageAttachments(
	ctx context.Context,
	unitUUID coreunit.UUID,
) (string, eventsource.Query[map[string]domainlife.Life]) {
	queryFunc := func(
		ctx context.Context, runner coredatabase.TxnRunner,
	) (map[string]domainlife.Life, error) {
		return st.getStorageAttachmentLifeForUnit(ctx, runner, unitUUID)
	}
	return "custom_storage_attachment_unit_uuid_lifecycle", queryFunc
}

// GetStorageAttachmentUUIDForUnit returns the UUID of the storage attachment for
// a given storage ID and network node UUID.
//
// The following errors may be returned:
// - [storageerrors.StorageInstanceNotFound] if the storage
// instance does not exist for the provided storage ID.
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [storageerrors.StorageAttachmentNotFound] if the storage attachment does
// not exist.
func (st *State) GetStorageAttachmentUUIDForUnit(
	ctx context.Context,
	storageInstanceID string,
	unitUUID coreunit.UUID,
) (storageprovisioning.StorageAttachmentUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	var (
		storageIDInput = storageID{ID: storageInstanceID}
		unitUUIDInput  = unitUUIDRef{UUID: unitUUID.String()}
		dbVal          entityUUID
	)
	stmt, err := st.Prepare(`
SELECT sa.uuid AS &entityUUID.uuid
FROM   storage_attachment sa
JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
WHERE  si.storage_id = $storageID.storage_id AND sa.unit_uuid = $unitUUIDRef.unit_uuid`,
		storageIDInput, unitUUIDInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkStorageInstanceUUIDByStorageID(
			ctx, tx, storageIDInput.ID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.Errorf(
				"storage instance %q does not exist", storageIDInput.ID,
			).Add(storageerrors.StorageInstanceNotFound)
		}

		exists, err = st.checkUnitExists(ctx, tx, unitUUID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.Errorf(
				"unit %q does not exist", unitUUIDInput.UUID,
			).Add(applicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, stmt, storageIDInput, unitUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage attachment for %q and unit %q does not exist",
				storageIDInput.ID, unitUUIDInput.UUID,
			).Add(storageerrors.StorageAttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf(
			"getting storage attachment UUID for %q and unit %q: %w",
			storageIDInput.ID, unitUUIDInput.UUID, err,
		)
	}
	return storageprovisioning.StorageAttachmentUUID(dbVal.UUID), nil
}

// NamespaceForStorageAttachment returns the change stream namespace
// for watching storage attachment changes.
func (st *State) NamespaceForStorageAttachment() string {
	return "custom_storage_attachment_entities_storage_attachment_uuid"
}
