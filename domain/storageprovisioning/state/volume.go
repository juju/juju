// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

// CheckVolumeForIDExists checks if a filesystem exists for the supplied
// volume ID. True is returned when a volume exists for the supplied id.
func (st *State) CheckVolumeForIDExists(
	ctx context.Context, volID string,
) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	io := volumeID{ID: volID}
	checkQuery, err := st.Prepare(`
SELECT &volumeID.*
FROM   storage_volume
WHERE  volume_id=$volumeID.volume_id
`, io)
	if err != nil {
		return false, errors.Capture(err)
	}

	var exists bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, checkQuery, io).Get(&io)
		if err == nil {
			exists = true
			return nil
		} else if errors.Is(err, sqlair.ErrNoRows) {
			exists = false
			return nil
		}
		return err
	})
	if err != nil {
		return false, errors.Capture(err)
	}

	return exists, nil
}

// checkVolumeAttachmentExists checks if a volume attachment for the provided
// UUID exists. True is returned when the volume attachment exists.
func (st *State) checkVolumeAttachmentExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storageprovisioning.VolumeAttachmentUUID,
) (bool, error) {
	vaUUIDInput := volumeAttachmentUUID{UUID: uuid.String()}

	checkQuery, err := st.Prepare(`
SELECT &volumeAttachmentUUID.*
FROM   storage_volume_attachment
WHERE  uuid = $volumeAttachmentUUID.uuid
`, vaUUIDInput)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkQuery, vaUUIDInput).Get(&vaUUIDInput)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// checkVolumeAttachmentPlanExists checks if a volume attachment for the
// provided UUID exists. True is returned when the volume attachment plan
// exists.
func (st *State) checkVolumeAttachmentPlanExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
) (bool, error) {
	io := entityUUID{UUID: uuid.String()}

	checkQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume_attachment_plan
WHERE  uuid = $entityUUID.uuid
`, io)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkQuery, io).Get(&io)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// checkVolumeExists checks if a volume for the provided uuid exists.
// Returning true when this case is satisfied.
func (st *State) checkVolumeExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid storageprovisioning.VolumeUUID,
) (bool, error) {
	io := volumeUUID{UUID: uuid.String()}

	checkQuery, err := st.Prepare(`
SELECT &volumeUUID.*
FROM   storage_volume
WHERE  uuid = $volumeUUID.uuid
`,
		io,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkQuery, io).Get(&io)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// GetVolumeAttachmentIDs returns the [storageprovisioning.VolumeAttachmentID]
// information for each volume attachment uuid supplied. If a uuid does not
// exist or isn't attached to either a machine or a unit then it will not exist
// in the result.
func (st *State) GetVolumeAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]storageprovisioning.VolumeAttachmentID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuidInputs := volumeAttachmentUUIDs(uuids)

	// To statisfy the unit name column of this union query a volume attachment
	// must be for a netnode uuid that is on a unit where that unit does not
	// share a netnode with a machine. If units are for machines they share a
	// netnode.
	q := `
SELECT &volumeAttachmentIDs.* FROM (
    SELECT sva.uuid,
           sv.volume_id,
           m.name AS machine_name,
           NULL AS unit_name
    FROM   storage_volume_attachment sva
    JOIN   storage_volume sv ON sva.storage_volume_uuid = sv.uuid
    JOIN   machine m ON sva.net_node_uuid = m.net_node_uuid
    WHERE  sva.uuid IN ($volumeAttachmentUUIDs[:])
    UNION
    SELECT      sva.uuid,
                sv.volume_id,
                NULL AS machine_name,
                u.name AS unit_name
    FROM        storage_volume_attachment sva
    JOIN        storage_volume sv ON sva.storage_volume_uuid = sv.uuid
    LEFT JOIN   machine m ON sva.net_node_uuid == m.net_node_uuid
    JOIN        unit u ON sva.net_node_uuid = u.net_node_uuid
    WHERE       sva.uuid IN ($volumeAttachmentUUIDs[:])
    AND         m.net_node_uuid IS NULL
)
`

	uuidToIDsStmt, err := st.Prepare(q, volumeAttachmentIDs{}, uuidInputs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var dbVals []volumeAttachmentIDs
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, uuidToIDsStmt, uuidInputs).GetAll(&dbVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := make(map[string]storageprovisioning.VolumeAttachmentID, len(dbVals))
	for _, v := range dbVals {
		id := storageprovisioning.VolumeAttachmentID{
			VolumeID: v.VolumeID,
		}
		if v.MachineName.Valid {
			machineName := coremachine.Name(v.MachineName.String)
			id.MachineName = &machineName
		}
		if v.UnitName.Valid {
			unitName := coreunit.Name(v.UnitName.String)
			id.UnitName = &unitName
		}

		rval[v.UUID] = id
	}
	return rval, nil
}

// GetVolumeAttachmentLife returns the current life value for a
// volume attachment uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided uuid.
func (st *State) GetVolumeAttachmentLife(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentUUID,
) (domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}

	var (
		uuidInput = volumeAttachmentUUID{UUID: uuid.String()}
		lifeDBVal entityLife
	)

	lifeQuery, err := st.Prepare(`
SELECT &entityLife.*
FROM   storage_volume_attachment
WHERE  uuid = $volumeAttachmentUUID.uuid
`,
		uuidInput, lifeDBVal,
	)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeQuery, uuidInput).Get(&lifeDBVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}
		return err
	})

	if err != nil {
		return 0, errors.Capture(err)
	}

	return domainlife.Life(lifeDBVal.LifeID), nil
}

// GetVolumeAttachmentLifeForNetNode returns a mapping of volume
// attachment uuid to the current life value for each machine provisioned
// volume attachment that is to be provisioned by the machine owning the
// supplied net node.
func (st *State) GetVolumeAttachmentLifeForNetNode(
	ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getVolumeAttachmentLifeForNetNode(ctx, db, netNodeUUID)
}

// getVolumeAttachmentLifeForNetNode returns a mapping of volume attachment uuid
// to the current life value for each machine provisioned volume attachment that
// is to be provisioned by the machine owning the supplied net node.
func (st *State) getVolumeAttachmentLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	uuid domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	netNodeInput := netNodeUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_volume_attachment
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNodeUUID.uuid
		`, attachmentLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var volAttachmentLives attachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", uuid)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&volAttachmentLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(volAttachmentLives.Iter), nil
}

// GetVolumeAttachmentPlanLifeForNetNode returns a mapping of volume
// attachment plan volume id to the current life value for each volume
// attachment plan. The volume id of the attachment plans is returned instead of
// the uuid because the caller for the watcher works off of this information.
func (st *State) GetVolumeAttachmentPlanLifeForNetNode(
	ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getVolumeAttachmentPlanLifeForNetNode(ctx, db, netNodeUUID)
}

// getVolumeAttachmentPlanLifeForNetNode returns a mapping of volume
// attachment plan volume id to the current life value for each volume
// attachment plan. The volume id of the attachment plan is returned instead of
// the uuid because the caller for the watcher works off of this information.
func (st *State) getVolumeAttachmentPlanLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	uuid domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	netNodeInput := netNodeUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sv.volume_id, svap.life_id) AS (&volumeAttachmentPlanLife.*)
FROM            storage_volume_attachment_plan svap
JOIN            storage_volume sv ON svap.storage_volume_uuid=sv.uuid
WHERE           svap.provision_scope_id=1
AND             svap.net_node_uuid=$netNodeUUID.uuid
		`, volumeAttachmentPlanLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var volAttachmentPlanLives volumeAttachmentPlanLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", uuid)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&volAttachmentPlanLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(volAttachmentPlanLives.Iter), nil
}

// GetVolumeAttachmentUUIDForVolumeNetNode returns the volume attachment uuid
// for the supplied volume uuid which is attached to the given net node
// uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists
// for the supplied uuid.
// - [networkerrors.NetNodeNotFound] when no net node exists for the supplied
// net node uuid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
func (st *State) GetVolumeAttachmentUUIDForVolumeNetNode(
	ctx context.Context,
	vUUID storageprovisioning.VolumeUUID,
	nodeUUID domainnetwork.NetNodeUUID,
) (storageprovisioning.VolumeAttachmentUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		vUUIDInput   = entityUUID{UUID: vUUID.String()}
		netNodeInput = netNodeUUID{UUID: nodeUUID.String()}
		dbVal        entityUUID
	)

	uuidQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume_attachment
WHERE  storage_volume_uuid = $entityUUID.uuid
AND    net_node_uuid = $netNodeUUID.uuid
	`,
		vUUIDInput, netNodeInput,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeExists(ctx, tx, vUUID)
		if err != nil {
			return errors.Errorf(
				"checking if volume %q exists: %w", vUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"volume %q does not exist", vUUID,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}

		exists, err = st.checkNetNodeExists(ctx, tx, nodeUUID)
		if err != nil {
			return errors.Errorf(
				"checking net node uuid %q exists: %w", nodeUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"net node %q does not exist", nodeUUID,
			).Add(networkerrors.NetNodeNotFound)
		}

		err = tx.Query(ctx, uuidQuery, vUUIDInput, netNodeInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment does not exist",
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return storageprovisioning.VolumeAttachmentUUID(dbVal.UUID), nil
}

// GetVolumeAttachmentPlanUUIDForVolumeNetNode returns the volume attachment
// uuid for the supplied volume uuid which is attached to the given net node
// uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// supplied uuid.
// - [networkerrors.NetNodeNotFound] when no net node exists for the supplied
// net node uuid.
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
// attachment plan exists for the supplied values.
func (st *State) GetVolumeAttachmentPlanUUIDForVolumeNetNode(
	ctx context.Context,
	vUUID storageprovisioning.VolumeUUID,
	nodeUUID domainnetwork.NetNodeUUID,
) (storageprovisioning.VolumeAttachmentPlanUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		vUUIDInput   = entityUUID{UUID: vUUID.String()}
		netNodeInput = netNodeUUID{UUID: nodeUUID.String()}
		dbVal        entityUUID
	)

	uuidQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume_attachment_plan
WHERE  storage_volume_uuid = $entityUUID.uuid
AND    net_node_uuid = $netNodeUUID.uuid
	`,
		vUUIDInput, netNodeInput,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeExists(ctx, tx, vUUID)
		if err != nil {
			return errors.Errorf(
				"checking if volume %q exists: %w", vUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"volume %q does not exist", vUUID,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}

		exists, err = st.checkNetNodeExists(ctx, tx, nodeUUID)
		if err != nil {
			return errors.Errorf(
				"checking net node uuid %q exists: %w", nodeUUID, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"net node %q does not exist", nodeUUID,
			).Add(networkerrors.NetNodeNotFound)
		}

		err = tx.Query(ctx, uuidQuery, vUUIDInput, netNodeInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment plan does not exist",
			).Add(storageprovisioningerrors.VolumeAttachmentPlanNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return storageprovisioning.VolumeAttachmentPlanUUID(dbVal.UUID), nil
}

// GetVolumeAttachment returns the volume attachment for the supplied volume
// attachment uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided uuid.
func (st *State) GetVolumeAttachment(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentUUID,
) (storageprovisioning.VolumeAttachment, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.VolumeAttachment{}, errors.Capture(err)
	}

	var (
		uuidInput = volumeAttachmentUUID{UUID: uuid.String()}
	)

	stmt, err := st.Prepare(`
SELECT &volumeAttachment.* FROM (
    SELECT     sv.volume_id,
               sva.life_id,
               sva.read_only,
               bd.name AS block_device_name,
               bd.bus_address AS block_device_bus_address,
               bd.uuid AS block_device_uuid
    FROM       storage_volume_attachment sva
    JOIN       storage_volume sv ON sv.uuid=sva.storage_volume_uuid
    LEFT JOIN  block_device bd ON bd.uuid=sva.block_device_uuid
    WHERE      sva.uuid = $volumeAttachmentUUID.uuid
)
`,
		uuidInput, volumeAttachment{},
	)
	if err != nil {
		return storageprovisioning.VolumeAttachment{}, errors.Capture(err)
	}

	devLinksStmt, err := st.Prepare(`
SELECT &entityName.*
FROM   block_device_link_device
WHERE  block_device_uuid = $entityUUID.uuid
`, entityName{}, entityUUID{})
	if err != nil {
		return storageprovisioning.VolumeAttachment{}, errors.Capture(err)
	}

	var va volumeAttachment
	var devLinks []entityName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uuidInput).Get(&va)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment does not exist",
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		} else if err != nil {
			return err
		}
		if va.BlockDeviceUUID == "" {
			return nil
		}
		blockDeviceUUID := entityUUID{
			UUID: va.BlockDeviceUUID,
		}
		err = tx.Query(ctx, devLinksStmt, blockDeviceUUID).GetAll(&devLinks)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return storageprovisioning.VolumeAttachment{}, errors.Capture(err)
	}

	retVal := storageprovisioning.VolumeAttachment{
		VolumeID:              va.VolumeID,
		ReadOnly:              va.ReadOnly,
		BlockDeviceName:       va.BlockDeviceName,
		BlockDeviceBusAddress: va.BlockDeviceBusAddress,
	}
	retVal.BlockDeviceLinks = make([]string, 0, len(devLinks))
	for _, v := range devLinks {
		retVal.BlockDeviceLinks = append(retVal.BlockDeviceLinks, v.Name)
	}
	return retVal, nil
}

// GetVolumeLife returns the current life value for a
// volume uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided uuid.
func (st *State) GetVolumeLife(
	ctx context.Context,
	uuid storageprovisioning.VolumeUUID,
) (domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}

	var (
		uuidInput = volumeUUID{UUID: uuid.String()}
		lifeDBVal entityLife
	)

	lifeQuery, err := st.Prepare(`
SELECT &entityLife.*
FROM   storage_volume
WHERE  uuid = $volumeUUID.uuid
`,
		uuidInput, lifeDBVal,
	)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lifeQuery, uuidInput).Get(&lifeDBVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}
		return err
	})

	if err != nil {
		return 0, errors.Capture(err)
	}

	return domainlife.Life(lifeDBVal.LifeID), nil
}

// GetVolumeLifeForNetNode returns a mapping of volume id to current life value
// for each machine provisioned volume that is to be provisioned by the machine
// owning the supplied net node.
func (st *State) GetVolumeLifeForNetNode(
	ctx context.Context, netNodeUUID domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getVolumeLifeForNetNode(ctx, db, netNodeUUID)
}

// getVolumeLifeForNetNode returns a mapping of volume id to current life value
// for each machine provisioned volume that is to be provisioned by the machine
// owning the supplied net node.
func (st *State) getVolumeLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	uuid domainnetwork.NetNodeUUID,
) (map[string]domainlife.Life, error) {
	netNodeInput := netNodeUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sv.volume_id, sv.life_id) AS (&volumeLife.*)
FROM            storage_volume sv
JOIN            storage_volume_attachment sva ON sv.uuid=sva.storage_volume_uuid
WHERE           sv.provision_scope_id=1
AND             sva.net_node_uuid=$netNodeUUID.uuid
		`, volumeLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var volLives volumeLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", uuid)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&volLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(volLives.Iter), nil
}

// GetVolumeUUIDForID returns the uuid for a volume with the supplied
// id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists
// for the provided volume uuid.
func (st *State) GetVolumeUUIDForID(
	ctx context.Context, vID string,
) (storageprovisioning.VolumeUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		idInput = volumeID{ID: vID}
		dbVal   entityUUID
	)
	uuidQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume
WHERE  volume_id = $volumeID.volume_id
`,
		idInput, dbVal,
	)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, uuidQuery, idInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume for id %q does not exist", vID,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return storageprovisioning.VolumeUUID(dbVal.UUID), nil
}

// GetVolume returns the volume information for the specified volume uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists
// for the provided volume uuid.
func (st *State) GetVolume(
	ctx context.Context, uuid storageprovisioning.VolumeUUID,
) (storageprovisioning.Volume, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.Volume{}, errors.Capture(err)
	}

	uuidInput := volumeUUID{UUID: uuid.String()}
	stmt, err := st.Prepare(`
SELECT &volume.*
FROM   storage_volume
WHERE  uuid = $volumeUUID.uuid
`,
		uuidInput, volume{},
	)
	if err != nil {
		return storageprovisioning.Volume{}, errors.Capture(err)
	}

	var vol volume
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, uuidInput).Get(&vol)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}
		return err
	})

	if err != nil {
		return storageprovisioning.Volume{}, errors.Capture(err)
	}

	retVal := storageprovisioning.Volume{
		VolumeID:   vol.VolumeID,
		ProviderID: vol.ProviderID,
		SizeMiB:    vol.SizeMiB,
		HardwareID: vol.HardwareID,
		WWN:        vol.WWN,
		Persistent: vol.Persistent,
	}
	return retVal, nil
}

// SetVolumeAttachmentProvisionedInfo sets on the provided volume the
// information about the provisioned volume attachment.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided volume attachment uuid.
// - [blockdeviceerrors.BlockDeviceNotFound] when no block device exists
// for a given block device uuid.
func (st *State) SetVolumeAttachmentProvisionedInfo(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentUUID,
	info storageprovisioning.VolumeAttachmentProvisionedInfo,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	input := volumeAttachmentProvisionedInfo{
		UUID:     uuid.String(),
		ReadOnly: info.ReadOnly,
	}
	if info.BlockDeviceUUID != nil {
		input.BlockDeviceUUID = sql.Null[string]{
			V:     info.BlockDeviceUUID.String(),
			Valid: true,
		}
	}

	stmt, err := st.Prepare(`
UPDATE storage_volume_attachment
SET    read_only = $volumeAttachmentProvisionedInfo.read_only,
       block_device_uuid = $volumeAttachmentProvisionedInfo.block_device_uuid
WHERE  uuid = $volumeAttachmentProvisionedInfo.uuid
`, input)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeAttachmentExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf(
				"volume attachment %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}

		if info.BlockDeviceUUID != nil {
			exists, err := st.checkBlockDeviceExists(
				ctx, tx, *info.BlockDeviceUUID)
			if err != nil {
				return err
			} else if !exists {
				return errors.Errorf(
					"block device %q does not exist", *info.BlockDeviceUUID,
				).Add(blockdeviceerrors.BlockDeviceNotFound)
			}
		}

		return tx.Query(ctx, stmt, input).Run()
	})
	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

func (st *State) checkBlockDeviceExists(
	ctx context.Context, tx *sqlair.TX, uuid blockdevice.BlockDeviceUUID,
) (bool, error) {
	io := entityUUID{UUID: uuid.String()}

	checkQuery, err := st.Prepare(`
SELECT &entityUUID.*
FROM   block_device
WHERE  uuid = $entityUUID.uuid
`, io)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkQuery, io).Get(&io)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// GetBlockDeviceForVolumeAttachment returns the uuid of the block device set
// for the specified volume attachment.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided uuid.
// - [storageprovisioningerrors.VolumeAttachmentWithoutBlockDevice] when the
// volume attachment does not yet have a block device.
func (st *State) GetBlockDeviceForVolumeAttachment(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
) (blockdevice.BlockDeviceUUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	io := volumeAttachmentProvisionedInfo{
		UUID: uuid.String(),
	}
	stmt, err := st.Prepare(`
SELECT &volumeAttachmentProvisionedInfo.*
FROM   storage_volume_attachment
WHERE  uuid = $volumeAttachmentProvisionedInfo.uuid
`, io)
	if err != nil {
		return "", err
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, io).Get(&io)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Capture(err)
	}

	if !io.BlockDeviceUUID.Valid {
		return "", errors.Errorf(
			"volume attachment %q without block device", uuid,
		).Add(storageprovisioningerrors.VolumeAttachmentWithoutBlockDevice)
	}

	return blockdevice.BlockDeviceUUID(io.BlockDeviceUUID.V), nil
}

// SetVolumeProvisionedInfo sets the provisioned information for the given
// volume.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists
// for the provided volume uuid.
func (st *State) SetVolumeProvisionedInfo(
	ctx context.Context,
	uuid storageprovisioning.VolumeUUID,
	info storageprovisioning.VolumeProvisionedInfo,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	input := volumeProvisionedInfo{
		UUID:       uuid.String(),
		ProviderID: info.ProviderID,
		HardwareID: info.HardwareID,
		WWN:        info.WWN,
		SizeMiB:    info.SizeMiB,
		Persistent: info.Persistent,
	}
	stmt, err := st.Prepare(`
UPDATE storage_volume
SET    provider_id = $volumeProvisionedInfo.provider_id,
       size_mib = $volumeProvisionedInfo.size_mib,
       hardware_id = $volumeProvisionedInfo.hardware_id,
       wwn = $volumeProvisionedInfo.wwn,
       persistent = $volumeProvisionedInfo.persistent
WHERE  uuid = $volumeProvisionedInfo.uuid
`,
		input,
	)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf(
				"volume %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}
		return tx.Query(ctx, stmt, input).Run()
	})

	if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// GetVolumeParams returns the volume params for the supplied uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for
// the uuid.
func (st *State) GetVolumeParams(
	ctx context.Context, uuid storageprovisioning.VolumeUUID,
) (storageprovisioning.VolumeParams, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.VolumeParams{}, errors.Capture(err)
	}

	var (
		input     = volumeUUID{UUID: uuid.String()}
		paramsVal volumeParams
	)

	paramsStmt, err := st.Prepare(`
SELECT &volumeParams.* FROM (
    SELECT    sv.volume_id,
              si.requested_size_mib,
              sp.type,
              sva.uuid AS volume_attachment_uuid
    FROM      storage_volume sv
    JOIN      storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
    JOIN      storage_instance si ON siv.storage_instance_uuid = si.uuid
    JOIN      storage_pool sp ON si.storage_pool_uuid = sp.uuid
    LEFT JOIN storage_volume_attachment sva ON sva.storage_volume_uuid = sv.uuid
    WHERE  sv.uuid = $volumeUUID.uuid
)
`,
		paramsVal, input,
	)
	if err != nil {
		return storageprovisioning.VolumeParams{}, errors.Capture(err)
	}

	poolAttributesStmt, err := st.Prepare(`
SELECT &storagePoolAttribute.*
FROM   storage_pool_attribute spa
JOIN   storage_pool sp ON spa.storage_pool_uuid = sp.uuid
JOIN   storage_instance si ON sp.uuid = si.storage_pool_uuid
JOIN   storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
JOIN   storage_volume sv ON siv.storage_volume_uuid = sv.uuid
WHERE  sv.uuid = $volumeUUID.uuid
`,
		storagePoolAttribute{}, input,
	)
	if err != nil {
		return storageprovisioning.VolumeParams{}, errors.Capture(err)
	}

	var attributeVals []storagePoolAttribute
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf("checking if volume %q exists: %w", uuid, err)
		}
		if !exists {
			return errors.Errorf(
				"volume %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeNotFound)
		}

		err = tx.Query(ctx, paramsStmt, input).Get(&paramsVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"volume is not associated with a storage instance",
			)
		} else if err != nil {
			return err
		}

		// It is ok to get no results. Not all storage pools have attributes.
		err = tx.Query(ctx, poolAttributesStmt, input).GetAll(&attributeVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf(
				"getting volume %q storage pool attributes: %w", uuid, err,
			)
		}
		return nil
	})

	if err != nil {
		return storageprovisioning.VolumeParams{}, errors.Capture(err)
	}

	attributesRval := make(map[string]string, len(attributeVals))
	for _, attr := range attributeVals {
		attributesRval[attr.Key] = attr.Value
	}

	retVal := storageprovisioning.VolumeParams{
		Attributes: attributesRval,
		ID:         paramsVal.VolumeID,
		Provider:   paramsVal.Type,
		SizeMiB:    paramsVal.RequestedSizeMiB,
	}
	if paramsVal.VolumeAttachmentUUID != "" {
		vaUUID := storageprovisioning.VolumeAttachmentUUID(
			paramsVal.VolumeAttachmentUUID)
		retVal.VolumeAttachmentUUID = &vaUUID
	}

	return retVal, nil
}

// GetVolumeAttachmentParams retrieves the attachment params for the given
// volume attachment.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied uuid.
func (st *State) GetVolumeAttachmentParams(
	ctx context.Context, uuid storageprovisioning.VolumeAttachmentUUID,
) (storageprovisioning.VolumeAttachmentParams, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.VolumeAttachmentParams{}, errors.Capture(err)
	}

	var (
		vaUUIDInput = volumeAttachmentUUID{UUID: uuid.String()}
		dbVal       volumeAttachmentParams
	)

	/*
			   id      parent  notused detail
			   20      0       0       SEARCH sva USING INDEX sqlite_autoindex_storage_volume_attachment_1 (uuid=?)
		       25      0       0       SEARCH sv USING INDEX sqlite_autoindex_storage_volume_1 (uuid=?)
		       30      0       0       SEARCH siv USING INDEX idx_storage_instance_volume (storage_volume_uuid=?)
		       38      0       0       SEARCH si USING INDEX sqlite_autoindex_storage_instance_1 (uuid=?)
		       43      0       0       SEARCH sp USING INDEX sqlite_autoindex_storage_pool_1 (uuid=?)
		       48      0       0       SEARCH sa USING COVERING INDEX idx_storage_attachment_unit_uuid_storage_instance_uuid (storage_instance_uuid=?)
		           	                   LEFT-JOIN
		       54      0       0       SEARCH u USING INDEX sqlite_autoindex_unit_1 (uuid=?) LEFT-JOIN
		       62      0       0       SEARCH cs USING INDEX sqlite_autoindex_charm_storage_1 (charm_uuid=? AND name=?) LEFT-JOIN
		       71      0       0       SEARCH m USING INDEX idx_machine_net_node (net_node_uuid=?) LEFT-JOIN
		       78      0       0       SEARCH mci USING INDEX sqlite_autoindex_machine_cloud_instance_1 (machine_uuid=?) LEFT-JOIN
	*/
	stmt, err := st.Prepare(`
SELECT &volumeAttachmentParams.* FROM (
    SELECT    sv.provider_id,
              mci.instance_id,
              cs.read_only,
              sp.type,
              m.name AS machine_name
    FROM      storage_volume_attachment sva
    JOIN      storage_volume sv ON sva.storage_volume_uuid = sv.uuid
    JOIN      storage_instance_volume siv ON sv.uuid = siv.storage_volume_uuid
    JOIN 	  storage_instance si ON siv.storage_instance_uuid = si.uuid
    JOIN      storage_pool sp ON si.storage_pool_uuid = sp.uuid
    LEFT JOIN storage_attachment sa ON si.uuid = sa.storage_instance_uuid
    LEFT JOIN unit u ON sa.unit_uuid = u.uuid
    LEFT JOIN charm_storage cs ON u.charm_uuid = cs.charm_uuid AND si.storage_name = cs.name
    LEFT JOIN machine m ON sva.net_node_uuid = m.net_node_uuid
    LEFT JOIN machine_cloud_instance mci ON m.uuid = mci.machine_uuid
    WHERE     sva.uuid = $volumeAttachmentUUID.uuid
)
`,
		vaUUIDInput, dbVal,
	)
	if err != nil {
		return storageprovisioning.VolumeAttachmentParams{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeAttachmentExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf(
				"checking if volume attachment %q exists: %w", uuid, err,
			)
		}
		if !exists {
			return errors.Errorf(
				"volume attachment %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}

		err = tx.Query(ctx, stmt, vaUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"volume attachment is not associated with a storage instance",
			)
		}
		return err
	})

	if err != nil {
		return storageprovisioning.VolumeAttachmentParams{}, errors.Capture(err)
	}

	retVal := storageprovisioning.VolumeAttachmentParams{
		MachineInstanceID: dbVal.InstanceID,
		Provider:          dbVal.Type,
		ProviderID:        dbVal.ProviderID,
		ReadOnly:          dbVal.ReadOnly,
	}
	if dbVal.MachineName != "" {
		m := coremachine.Name(dbVal.MachineName)
		retVal.Machine = &m
	}

	return retVal, nil
}

// GetVolumeAttachmentPlan gets the volume attachment plan for the provided
// uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotPlanFound] when no volume
// attachment plan exists for the provided uuid.
func (st *State) GetVolumeAttachmentPlan(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
) (storageprovisioning.VolumeAttachmentPlan, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.VolumeAttachmentPlan{}, errors.Capture(err)
	}

	input := entityUUID{
		UUID: uuid.String(),
	}
	planStmt, err := st.Prepare(`
SELECT &volumeAttachmentPlan.*
FROM   storage_volume_attachment_plan
WHERE  storage_volume_attachment_plan.uuid = $entityUUID.uuid
`, input, volumeAttachmentPlan{})
	if err != nil {
		return storageprovisioning.VolumeAttachmentPlan{}, errors.Capture(err)
	}

	attrStmt, err := st.Prepare(`
SELECT &volumeAttachmentPlanAttr.*
FROM   storage_volume_attachment_plan_attr
WHERE  attachment_plan_uuid = $entityUUID.uuid
`, input, volumeAttachmentPlanAttr{})
	if err != nil {
		return storageprovisioning.VolumeAttachmentPlan{}, errors.Capture(err)
	}

	var plan volumeAttachmentPlan
	var attrs []volumeAttachmentPlanAttr
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, planStmt, input).Get(&plan)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment plan %q not found", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentPlanNotFound)
		} else if err != nil {
			return err
		}

		err = tx.Query(ctx, attrStmt, input).GetAll(&attrs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}

		return nil
	})
	if err != nil {
		return storageprovisioning.VolumeAttachmentPlan{}, errors.Capture(err)
	}

	retVal := storageprovisioning.VolumeAttachmentPlan{
		Life: plan.Life,
	}
	switch plan.DeviceTypeID {
	case 0:
		retVal.DeviceType = storageprovisioning.PlanDeviceTypeLocal
	case 1:
		retVal.DeviceType = storageprovisioning.PlanDeviceTypeISCSI
	}
	if len(attrs) == 0 {
		return retVal, nil
	}

	retVal.DeviceAttributes = make(map[string]string, len(attrs))
	for _, v := range attrs {
		retVal.DeviceAttributes[v.Key] = v.Value
	}

	return retVal, nil
}

// CreateVolumeAttachmentPlan creates a volume attachment plan for the
// provided volume attachment uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided uuid.
// - [storageprovisioningerrors.VolumeAttachmentPlanAlreadyExists] when a
// volume attachment plan already exists for the given volume attachnment.
func (st *State) CreateVolumeAttachmentPlan(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
	attachmentUUID storageprovisioning.VolumeAttachmentUUID,
	deviceType storageprovisioning.PlanDeviceType,
	attrs map[string]string,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	vap := volumeAttachmentPlan{
		UUID: uuid.String(),
		Life: domainlife.Alive,
	}
	switch deviceType {
	case storageprovisioning.PlanDeviceTypeLocal:
		vap.DeviceTypeID = 0
	case storageprovisioning.PlanDeviceTypeISCSI:
		vap.DeviceTypeID = 1
	}
	va := volumeAttachmentInfo{
		UUID: attachmentUUID.String(),
	}
	vaStmt, err := st.Prepare(`
SELECT &volumeAttachmentInfo.*
FROM   storage_volume_attachment
WHERE  uuid = $volumeAttachmentInfo.uuid
`, va)
	if err != nil {
		return errors.Capture(err)
	}

	planCheckStmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume_attachment_plan
WHERE  storage_volume_uuid = $volumeAttachmentInfo.storage_volume_uuid AND
       net_node_uuid = $volumeAttachmentInfo.net_node_uuid
`, entityUUID{}, va)
	if err != nil {
		return errors.Capture(err)
	}

	planInsertStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment_plan (uuid, storage_volume_uuid,
                                            net_node_uuid, life_id,
                                            device_type_id, provision_scope_id)
VALUES      ($volumeAttachmentPlan.uuid,
             $volumeAttachmentInfo.storage_volume_uuid,
             $volumeAttachmentInfo.net_node_uuid,
             $volumeAttachmentPlan.life_id,
             $volumeAttachmentPlan.device_type_id,
             1)
`, va, vap)
	if err != nil {
		return errors.Capture(err)
	}

	attrInsertStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment_plan_attr (attachment_plan_uuid, key, value)
VALUES      ($volumeAttachmentPlanAttr.*)
`, volumeAttachmentPlanAttr{})
	if err != nil {
		return errors.Capture(err)
	}

	dbAttrs := make([]volumeAttachmentPlanAttr, 0, len(attrs))
	for k, v := range attrs {
		dbAttrs = append(dbAttrs, volumeAttachmentPlanAttr{
			AttachmentPlanUUID: uuid.String(),
			Key:                k,
			Value:              v,
		})
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, vaStmt, va).Get(&va)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment %q not found", attachmentUUID,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		} else if err != nil {
			return err
		}

		existing := entityUUID{}
		err = tx.Query(ctx, planCheckStmt, va).Get(&existing)
		if err == nil {
			return errors.Errorf(
				"volume attachment plan already exists",
			).Add(storageprovisioningerrors.VolumeAttachmentPlanAlreadyExists)
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}

		err = tx.Query(ctx, planInsertStmt, va, vap).Run()
		if err != nil {
			return err
		}

		if len(dbAttrs) > 0 {
			err = tx.Query(ctx, attrInsertStmt, dbAttrs).Run()
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetVolumeAttachmentPlanProvisionedInfo sets on the provided volume attachment
// plan information.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
// attachment plan exists for the provided uuid.
func (st *State) SetVolumeAttachmentPlanProvisionedInfo(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
	info storageprovisioning.VolumeAttachmentPlanProvisionedInfo,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	vap := volumeAttachmentPlan{
		UUID: uuid.String(),
	}
	switch info.DeviceType {
	case storageprovisioning.PlanDeviceTypeLocal:
		vap.DeviceTypeID = 0
	case storageprovisioning.PlanDeviceTypeISCSI:
		vap.DeviceTypeID = 1
	}

	updatePlanStmt, err := st.Prepare(`
UPDATE storage_volume_attachment_plan
SET    device_type_id = $volumeAttachmentPlan.device_type_id
WHERE  uuid = $volumeAttachmentPlan.uuid
`, vap)
	if err != nil {
		return errors.Capture(err)
	}

	attrDeleteStmt, err := st.Prepare(`
DELETE FROM storage_volume_attachment_plan_attr
WHERE       attachment_plan_uuid = $volumeAttachmentPlan.uuid
`, vap)
	if err != nil {
		return errors.Capture(err)
	}

	attrInsertStmt, err := st.Prepare(`
INSERT INTO storage_volume_attachment_plan_attr (attachment_plan_uuid, key, value)
VALUES      ($volumeAttachmentPlanAttr.*)
`, volumeAttachmentPlanAttr{})
	if err != nil {
		return errors.Capture(err)
	}

	dbAttrs := make([]volumeAttachmentPlanAttr, 0, len(info.DeviceAttributes))
	for k, v := range info.DeviceAttributes {
		dbAttrs = append(dbAttrs, volumeAttachmentPlanAttr{
			AttachmentPlanUUID: uuid.String(),
			Key:                k,
			Value:              v,
		})
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeAttachmentPlanExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf(
				"volume attachment plan %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentPlanNotFound)
		}

		err = tx.Query(ctx, updatePlanStmt, vap).Run()
		if err != nil {
			return err
		}

		err = tx.Query(ctx, attrDeleteStmt, vap).Run()
		if err != nil {
			return err
		}

		if len(dbAttrs) > 0 {
			err = tx.Query(ctx, attrInsertStmt, dbAttrs).Run()
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetVolumeAttachmentPlanProvisionedBlockDevice sets on the provided
// volume attachment plan the information about the provisioned block device.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentPlanNotFound] when no volume
// attachment plan exists for the provided uuid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the provided attachment plan.
// - [blockdeviceerrors.BlockDeviceNotFound] when no block device exists for the
// provided block device uuid.
func (st *State) SetVolumeAttachmentPlanProvisionedBlockDevice(
	ctx context.Context,
	uuid storageprovisioning.VolumeAttachmentPlanUUID,
	blockDeviceUUID blockdevice.BlockDeviceUUID,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	input := volumeAttachmentPlanUUID{
		UUID: uuid.String(),
	}
	bdUUID := entityUUID{
		UUID: blockDeviceUUID.String(),
	}
	vaUUID := volumeAttachmentUUID{}

	vaUUIDStmt, err := st.Prepare(`
SELECT sva.uuid AS &volumeAttachmentUUID.uuid
FROM   storage_volume_attachment_plan svap
JOIN   storage_volume_attachment sva ON sva.storage_volume_uuid=svap.storage_volume_uuid AND
                                        sva.net_node_uuid=svap.net_node_uuid
WHERE  svap.uuid = $volumeAttachmentPlanUUID.uuid
`, input, vaUUID)
	if err != nil {
		return errors.Capture(err)
	}

	updateStmt, err := st.Prepare(`
UPDATE storage_volume_attachment
SET    block_device_uuid = $entityUUID.uuid
WHERE  uuid = $volumeAttachmentUUID.uuid
`, vaUUID, bdUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkVolumeAttachmentPlanExists(ctx, tx, uuid)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf(
				"volume attachment plan %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentPlanNotFound)
		}

		exists, err = st.checkBlockDeviceExists(ctx, tx, blockDeviceUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf(
				"block device %q does not exist", blockDeviceUUID,
			).Add(blockdeviceerrors.BlockDeviceNotFound)
		}

		err = tx.Query(ctx, vaUUIDStmt, input).Get(&vaUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment missing for volume attachment plan %q", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		} else if err != nil {
			return err
		}

		return tx.Query(ctx, updateStmt, vaUUID, bdUUID).Run()
	})
	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// GetVolumeAttachmentUUIDForStorageID returns the volume
// attachment uuid for the supplied storage id.
//
// The following errors may be returned:
// - [storageprovisioningerrors.StorageInstanceNotFound] when no storage
// instance exists for the supplied storage instance id.
// - [networkerrors.NetNodeNotFound] when no net node exists for the supplied
// net node uuid.
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied values.
func (st *State) GetVolumeAttachmentUUIDForStorageID(
	ctx context.Context,
	storageInstanceID string,
	nodeUUID string,
) (string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var (
		storageIDInput = storageID{ID: storageInstanceID}
		nodeUUIDInput  = netNodeUUID{UUID: nodeUUID}
		dbVal          volumeAttachmentUUID
	)

	stmt, err := st.Prepare(`
SELECT    sva.uuid AS &volumeAttachmentUUID.uuid
FROM      storage_volume_attachment sva
JOIN      storage_volume sv ON sva.storage_volume_uuid = sv.uuid
LEFT JOIN storage_instance_volume siv ON sv.uuid = siv.storage_volume_uuid
LEFT JOIN storage_instance si ON siv.storage_instance_uuid = si.uuid
WHERE     si.storage_id = $storageID.storage_id
AND       sva.net_node_uuid = $netNodeUUID.uuid
`, storageIDInput, nodeUUIDInput, dbVal)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if exists, err := st.checkNetNodeExists(ctx, tx, domainnetwork.NetNodeUUID(nodeUUID)); err != nil {
			return errors.Errorf(
				"checking if net node %q exists: %w", nodeUUID, err,
			)
		} else if !exists {
			return errors.Errorf(
				"net node %q does not exist", nodeUUID,
			).Add(networkerrors.NetNodeNotFound)
		}

		if exists, err := st.checkStorageInstanceUUIDByStorageID(ctx, tx, storageInstanceID); err != nil {
			return errors.Errorf(
				"checking if storage instance %q exists: %w", storageInstanceID, err,
			)
		} else if !exists {
			return errors.Errorf(
				"storage instance %q does not exist", storageInstanceID,
			).Add(storageprovisioningerrors.StorageInstanceNotFound)
		}

		err = tx.Query(ctx, stmt, storageIDInput, nodeUUIDInput).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment for storage instance %q and net node %q does not exist",
				storageInstanceID, nodeUUID,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}
		return err
	})

	if err != nil {
		return "", errors.Capture(err)
	}

	return dbVal.UUID, nil
}

// GetVolumeAttachmentInfo retrieves information about the volume attachment
// with volume information included.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeAttachmentNotFound] when no volume
// attachment exists for the supplied uuid.
func (st *State) GetVolumeAttachmentInfo(
	ctx context.Context,
	uuid string,
) (domainstorageprovisioning.VolumeAttachmentInfo, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return domainstorageprovisioning.VolumeAttachmentInfo{}, errors.Capture(err)
	}

	var (
		input = volumeAttachmentUUID{UUID: uuid}
		dbVal volumeAttachmentData
	)

	stmt, err := st.Prepare(`
SELECT &volumeAttachmentData.* FROM (
	SELECT    sv.hardware_id,
              sv.wwn,
              bd.name AS block_device_name,
              first_value(bdld.name) OVER bdld_first AS block_device_link
    FROM      storage_volume_attachment sva
    JOIN      storage_volume sv ON sva.storage_volume_uuid = sv.uuid
    LEFT JOIN block_device bd ON bd.uuid=sva.block_device_uuid
    LEFT JOIN block_device_link_device bdld ON bdld.block_device_uuid=bd.uuid
    WHERE     sva.uuid = $volumeAttachmentUUID.uuid
	WINDOW    bdld_first AS (PARTITION BY bdld.block_device_uuid ORDER BY bdld.name)
)`,
		input, dbVal,
	)
	if err != nil {
		return domainstorageprovisioning.VolumeAttachmentInfo{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, input).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"volume attachment %q does not exist", uuid,
			).Add(storageprovisioningerrors.VolumeAttachmentNotFound)
		}
		return err
	})

	if err != nil {
		return domainstorageprovisioning.VolumeAttachmentInfo{}, errors.Capture(err)
	}

	return domainstorageprovisioning.VolumeAttachmentInfo{
		HardwareID:      dbVal.HardwareId,
		WWN:             dbVal.WWN,
		BlockDeviceName: dbVal.BlockDeviceName,
		BlockDeviceLink: dbVal.BlockDeviceLink,
	}, nil
}

// InitialWatchStatementMachineProvisionedVolumes returns both the namespace for
// watching volume life changes where the volume is machine provisioned. On top
// of this the initial query for getting all volumes in the model that are
// machine provisioned is returned.
//
// Only volumes that can be provisioned by the machine connected to the
// supplied net node will be emitted.
func (st *State) InitialWatchStatementMachineProvisionedVolumes(
	netNodeUUID domainnetwork.NetNodeUUID,
) (string, eventsource.Query[map[string]domainlife.Life]) {
	query := func(
		ctx context.Context,
		db database.TxnRunner,
	) (map[string]domainlife.Life, error) {
		return st.getVolumeLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_life_machine_provisioning", query
}

// InitialWatchStatementModelProvisionedVolumes returns both the namespace for
// watching volume life changes where the volume is model provisioned. On top of
// this the initial query for getting all volumes in the model that are model
// provisioned is returned.
func (st *State) InitialWatchStatementModelProvisionedVolumes() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
			SELECT &volumeID.* FROM storage_volume WHERE provision_scope_id=0
		`, volumeID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var volIDs []volumeID
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&volIDs)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		rval := make([]string, 0, len(volIDs))
		for _, v := range volIDs {
			rval = append(rval, v.ID)
		}
		return rval, nil
	}
	return "storage_volume_life_model_provisioning", query
}

// InitialWatchStatementMachineProvisionedVolumeAttachments returns both the
// namespace for watching volume attachment life changes where the volume
// attachment is machine provisioned. On top of this the initial query for
// getting all volume attachments in the model that are machine provisioned is
// returned.
//
// Only volume attachments that can be provisioned by the machine connected to
// the supplied net node will be emitted.
func (st *State) InitialWatchStatementMachineProvisionedVolumeAttachments(
	netNodeUUID domainnetwork.NetNodeUUID,
) (string, eventsource.Query[map[string]domainlife.Life]) {
	query := func(ctx context.Context,
		db database.TxnRunner,
	) (map[string]domainlife.Life, error) {
		return st.getVolumeAttachmentLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_attachment_life_machine_provisioning", query
}

// InitialWatchStatementModelProvisionedVolumeAttachments returns both the
// namespace for watching volume attachment life changes where the volume
// attachment is model provisioned. On top of this the initial query for getting
// all volume attachments in the model that are model provisioned is returned.
func (st *State) InitialWatchStatementModelProvisionedVolumeAttachments() (
	string, eventsource.NamespaceQuery,
) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT &entityUUID.*
FROM   storage_volume_attachment
WHERE  provision_scope_id=0
		`, entityUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var volAttachmentUUIDs []entityUUID
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&volAttachmentUUIDs)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		rval := make([]string, 0, len(volAttachmentUUIDs))
		for _, v := range volAttachmentUUIDs {
			rval = append(rval, v.UUID)
		}
		return rval, nil
	}
	return "storage_volume_attachment_life_model_provisioning", query
}

// InitialWatchStatementVolumeAttachmentPlans returns both the namespace for
// watching volume attachment plan life changes. On top of this the initial
// query for getting all volume attachment plan volume ids in the model that
// are for the given net node uuid.
func (st *State) InitialWatchStatementVolumeAttachmentPlans(
	netNodeUUID domainnetwork.NetNodeUUID,
) (string, eventsource.Query[map[string]domainlife.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]domainlife.Life, error) {
		return st.getVolumeAttachmentPlanLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_attachment_plan_life_machine_provisioning", query
}
