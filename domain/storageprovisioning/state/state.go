// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
}

// NewState creates and returns a new [State] for provisioning storage in the
// model.
func NewState(
	factory database.TxnRunnerFactory,
) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
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
	db, err := st.DB()
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
	db, err := st.DB()
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
	db, err := st.DB()
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
func (st *State) GetStorageResourceTagInfoForApplication(
	ctx context.Context,
	appUUID application.ID,
	resourceTagModelConfigKey string,
) (storageprovisioning.ResourceTagInfo, error) {
	db, err := st.DB()
	if err != nil {
		return storageprovisioning.ResourceTagInfo{}, errors.Capture(err)
	}

	type modelConfigKey struct {
		Key string `db:"key"`
	}
	appUUIDInput := entityUUID{UUID: appUUID.String()}
	modelConfigKeyInput := modelConfigKey{Key: resourceTagModelConfigKey}

	appNameStmt, err := st.Prepare(`
SELECT name AS &resourceTagInfo.application_name
FROM application
WHERE uuid = $entityUUID.uuid
`, resourceTagInfo{}, appUUIDInput)
	if err != nil {
		return storageprovisioning.ResourceTagInfo{}, errors.Capture(err)
	}
	resourceTagStmt, err := st.Prepare(`
SELECT value AS &resourceTagInfo.resource_tags
FROM model_config
WHERE key = $modelConfigKey.key
`, resourceTagInfo{}, modelConfigKeyInput)
	if err != nil {
		return storageprovisioning.ResourceTagInfo{}, errors.Capture(err)
	}
	modelInfoStmt, err := st.Prepare(`
SELECT uuid AS &resourceTagInfo.model_uuid, &resourceTagInfo.controller_uuid
FROM model
`, resourceTagInfo{})
	if err != nil {
		return storageprovisioning.ResourceTagInfo{}, errors.Capture(err)
	}

	output := resourceTagInfo{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, appNameStmt, appUUIDInput).Get(&output)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("application %q does not exist", appUUID)
		} else if err != nil {
			return err
		}
		err = tx.Query(ctx, resourceTagStmt, modelConfigKeyInput).Get(&output)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return tx.Query(ctx, modelInfoStmt).Get(&output)
	})
	if err != nil {
		return storageprovisioning.ResourceTagInfo{}, errors.Capture(err)
	}

	return storageprovisioning.ResourceTagInfo{
		BaseResourceTags: output.ResourceTags,
		ModelUUID:        output.ModelUUID,
		ControllerUUID:   output.ControllerUUID,
		ApplicationName:  output.ApplicationName,
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
	uuid string,
) (bool, error) {
	input := unitUUID{UUID: uuid}

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

// checkApplicationExists checks if the provided application uuid exists in the
// database during a txn. False is returned when the application does not exist.
func (st *State) checkApplicationExists(
	ctx context.Context,
	tx *sqlair.TX,
	appUUID application.ID,
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

// GetStorageIDsForUnit returns the storage IDs for the given unit UUID.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] when no unit exists for the supplied unit UUID.
// - [corestorage.InvalidStorageID] when the provided unit UUID is invalid.
func (s *State) GetStorageIDsForUnit(ctx context.Context, unitUUID string) ([]corestorage.ID, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := storageAttachment{
		UnitUUID: unitUUID,
	}
	stmt, err := s.Prepare(`
SELECT &storageAttachment.*
FROM   storage_attachment sa
JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
WHERE  unit_uuid = $storageAttachment.unit_uuid`, input)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var attachments storageAttachments
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if exists, err := s.checkUnitExists(ctx, tx, unitUUID); err != nil {
			return errors.Capture(err)
		} else if !exists {
			return errors.Errorf("unit %q does not exist", unitUUID).Add(
				applicationerrors.UnitNotFound,
			)
		}

		err := tx.Query(ctx, stmt, input).GetAll(&attachments)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q does not exist", unitUUID).Add(
				applicationerrors.UnitNotFound,
			)
		}
		return err
	})

	if err != nil {
		return nil, errors.Capture(err)
	}
	var storageIDs []corestorage.ID
	for _, attachment := range attachments {
		id := corestorage.ID(attachment.StorageID)
		if err := id.Validate(); err != nil {
			// This should not happen, but if it does, we return an error.
			return nil, errors.Errorf(
				"invalid storage ID %q for unit %q: %w", id, unitUUID, err,
			).Add(corestorage.InvalidStorageID)
		}
		storageIDs = append(storageIDs, id)
	}
	return storageIDs, nil
}

// GetAttachmentLife retrieves the life of a storage attachment for a unit.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound] when no unit exists for the supplied unit UUID.
// - [storageprovisioningerrors.AttachmentNotFound] when the storage attachment does not exist for the unit.
func (s *State) GetAttachmentLife(ctx context.Context, unitUUID, storageID string) (domainlife.Life, error) {
	db, err := s.DB()
	if err != nil {
		return -1, errors.Capture(err)
	}
	attachment := storageAttachmentLife{
		UnitUUID:  unitUUID,
		StorageID: storageID,
	}
	stmt, err := s.Prepare(`
SELECT (si.life_id) AS (&storageAttachmentLife.*)
FROM   storage_attachment sa
JOIN   storage_instance si ON sa.storage_instance_uuid = si.uuid
WHERE  sa.unit_uuid = $storageAttachmentLife.unit_uuid
AND    si.storage_id = $storageAttachmentLife.storage_id`, attachment)
	if err != nil {
		return -1, errors.Capture(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if exists, err := s.checkUnitExists(ctx, tx, unitUUID); err != nil {
			return errors.Capture(err)
		} else if !exists {
			return errors.Errorf("unit %q does not exist", unitUUID).Add(
				applicationerrors.UnitNotFound,
			)
		}

		err := tx.Query(ctx, stmt, attachment).Get(&attachment)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"storage attachment %q for unit %q does not exist", storageID, unitUUID,
			).Add(storageprovisioningerrors.AttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return -1, errors.Capture(err)
	}
	return domainlife.Life(attachment.Life), nil
}
