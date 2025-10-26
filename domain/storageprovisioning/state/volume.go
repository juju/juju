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
	domainblockdevice "github.com/juju/juju/domain/blockdevice"
	blockdeviceerrors "github.com/juju/juju/domain/blockdevice/errors"
	domainlife "github.com/juju/juju/domain/life"
	domainmachineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/domain/storageprovisioning/internal"
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

// GetMachineModelProvisionedVolumeAttachmentParams returns the volume
// attachment parameters for all attachments onto a machine that are provisioned
// by the model. Should the machine have no volumes that are model provisioned
// then an empty result is returned.
//
// The following errors may be returned:
// - [domainmachineerrors.MachineNotFound] when no machine exists for the uuid.
func (st *State) GetMachineModelProvisionedVolumeAttachmentParams(
	ctx context.Context, uuid coremachine.UUID,
) ([]internal.MachineVolumeAttachmentProvisioningParams, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := entityUUID{UUID: uuid.String()}

	paramsStmt, err := st.Prepare(`
SELECT &machineVolumeAttachmentProvisioningParams.* FROM (
    SELECT    sv.volume_id,
              sp.type AS provider_type,
              sva.block_device_uuid,
              sva.read_only
    FROM      storage_volume_attachment sva
    JOIN      storage_volume sv ON sv.uuid = sva.storage_volume_uuid
    JOIN      machine m ON sva.net_node_uuid = m.net_node_uuid
    JOIN      storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
    JOIN      storage_instance si ON siv.storage_instance_uuid = si.uuid
    JOIN      storage_pool sp ON si.storage_pool_uuid = sp.uuid
    WHERE     m.uuid = $entityUUID.uuid
    AND       sva.provision_scope_id = 0
)
`,
		machineVolumeAttachmentProvisioningParams{}, input)
	if err != nil {
		return nil, errors.Errorf("preparing machine volume attachment params statment: %w", err)
	}

	var volumeAttachDBParams []machineVolumeAttachmentProvisioningParams
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkMachineExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf("checking if machine %q exists: %w", uuid, err)
		}
		if !exists {
			return errors.Errorf(
				"machine %q does not exist in the model", uuid,
			).Add(domainmachineerrors.MachineNotFound)
		}

		err = tx.Query(ctx, paramsStmt, input).GetAll(&volumeAttachDBParams)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	retVal := make(
		[]internal.MachineVolumeAttachmentProvisioningParams,
		0, len(volumeAttachDBParams),
	)
	for _, dbVal := range volumeAttachDBParams {
		params := internal.MachineVolumeAttachmentProvisioningParams{
			Provider: dbVal.ProviderType,
			ReadOnly: dbVal.ReadOnly.V,
			VolumeID: dbVal.VolumeID,
		}
		if dbVal.BlockDeviceUUID.Valid {
			blockDeviceUUID := domainblockdevice.BlockDeviceUUID(dbVal.BlockDeviceUUID.V)
			params.BlockDeviceUUID = &blockDeviceUUID
		}
		retVal = append(retVal, params)
	}

	return retVal, nil
}

// GetMachineModelProvisionedVolumeParams returns the volume parameters for all
// volumes of a machine that are provisioned by the model. The decision of if a
// volume in the model is for a machine is made by checking if the volume has an
// attachment onto the machine. Should the machine have no volumes that are
// model provisioned then an empty result is returned.
//
// The following errors may be returned:
// - [domainmachineerrors.MachineNotFound] when no machine exists for the uuid.
func (st *State) GetMachineModelProvisionedVolumeParams(
	ctx context.Context, uuid coremachine.UUID,
) ([]internal.MachineVolumeProvisioningParams, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	input := entityUUID{UUID: uuid.String()}

	paramsStmt, err := st.Prepare(`
SELECT &machineVolumeProvisioningParams.* FROM (
    SELECT    sv.volume_id,
              sv.size_mib,
              si.requested_size_mib,
              si.storage_id,
              sp.type AS provider_type,
              sp.uuid AS storage_pool_uuid,
              u.name AS storage_unit_owner_name
    FROM      storage_volume sv
    JOIN      storage_volume_attachment sva ON sva.storage_volume_uuid = sv.uuid
    JOIN      machine m ON sva.net_node_uuid = m.net_node_uuid
    JOIN      storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
    JOIN      storage_instance si ON siv.storage_instance_uuid = si.uuid
    JOIN      storage_pool sp ON si.storage_pool_uuid = sp.uuid
    -- NOTE (tlm) This left join is about establishing if the storage instance
    -- is fulfilling shared charm storage. If it is not then then we assume that
    -- at most only one storage attachment is made to the storage instance.
    LEFT JOIN (storage_attachment sa
               JOIN  storage_instance si2 ON sa.storage_instance_uuid = si2.uuid
               JOIN  unit u ON sa.unit_uuid = u.uuid
               JOIN  charm_storage cs ON u.charm_uuid = cs.charm_uuid
                                     AND si2.storage_name = cs.name
              ) ON sa.storage_instance_uuid = si.uuid
               AND cs.shared = FALSE
    WHERE  m.uuid = $entityUUID.uuid
    AND    sv.provision_scope_id = 0
)
`,
		machineVolumeProvisioningParams{}, input,
	)
	if err != nil {
		return nil, errors.Errorf("preparing machine volume params statment: %w", err)
	}

	// Because we are getting storage pool attributes for many volumes they may
	// or may not share the same volume. If they do share a volume then we have
	// a group by clause to make sure we only get each attribute once.
	poolAttributesStmt, err := st.Prepare(`
SELECT &storagePoolAttributeWithUUID.* FROM (
    SELECT spa.storage_pool_uuid, spa.key, spa.value
    FROM   storage_pool_attribute spa
    JOIN   storage_pool sp ON spa.storage_pool_uuid = sp.uuid
    JOIN   storage_instance si ON sp.uuid = si.storage_pool_uuid
    JOIN   storage_instance_volume siv ON si.uuid = siv.storage_instance_uuid
    JOIN   storage_volume sv ON siv.storage_volume_uuid = sv.uuid
    JOIN   storage_volume_attachment sva ON sva.storage_volume_uuid = sv.uuid
    JOIN   machine m ON sva.net_node_uuid = m.net_node_uuid
    WHERE  m.uuid = $entityUUID.uuid
    AND    sv.provision_scope_id = 0
    GROUP  BY spa.storage_pool_uuid, spa.key
)
`,
		storagePoolAttributeWithUUID{}, input,
	)
	if err != nil {
		return nil, errors.Errorf("preparing ")
	}

	var (
		attributeDBVals []storagePoolAttributeWithUUID
		volumeDBParams  []machineVolumeProvisioningParams
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkMachineExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf("checking if machine %q exists: %w", uuid, err)
		}
		if !exists {
			return errors.Errorf(
				"machine %q does not exist in the model", uuid,
			).Add(domainmachineerrors.MachineNotFound)
		}

		err = tx.Query(ctx, paramsStmt, input).GetAll(&volumeDBParams)
		if errors.Is(err, sqlair.ErrNoRows) {
			// There are no volume parameters for this machine.
			return nil
		} else if err != nil {
			return err
		}

		// It is ok to get no results. Not all storage pools have attributes.
		err = tx.Query(ctx, poolAttributesStmt, input).GetAll(&attributeDBVals)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf(
				"getting storage pool attributes for machine %q volume params: %w",
				uuid, err,
			)
		}
		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	poolAttributeMap := make(map[string][]storagePoolAttributeWithUUID)
	for _, attr := range attributeDBVals {
		attrs := poolAttributeMap[attr.StoragePoolUUID]
		poolAttributeMap[attr.StoragePoolUUID] = append(attrs, attr)
	}

	retVal := make([]internal.MachineVolumeProvisioningParams, 0, len(volumeDBParams))
	for _, dbParams := range volumeDBParams {
		attributes := make(map[string]string, len(poolAttributeMap[dbParams.StoragePoolUUID]))
		for _, attr := range poolAttributeMap[dbParams.StoragePoolUUID] {
			attributes[attr.Key] = attr.Value
		}

		params := internal.MachineVolumeProvisioningParams{
			Attributes:       attributes,
			ID:               dbParams.VolumeID,
			Provider:         dbParams.ProviderType,
			RequestedSizeMiB: dbParams.RequestedSizeMiB,
			SizeMiB:          dbParams.SizeMiB,
		}
		if dbParams.StorageUnitOwnerName.Valid {
			params.StorageOwnerUnitName = &dbParams.StorageUnitOwnerName.V
		}
		retVal = append(retVal, params)
	}

	return retVal, nil
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
		paramsVal volumeProvisioningParams
	)

	paramsStmt, err := st.Prepare(`
SELECT &volumeProvisioningParams.* FROM (
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

// GetVolumeRemovalParams returns the parameters needed to removal a volume.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for
// the uuid.
func (st *State) GetVolumeRemovalParams(
	ctx context.Context, uuid storageprovisioning.VolumeUUID,
) (storageprovisioning.VolumeRemovalParams, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return storageprovisioning.VolumeRemovalParams{}, errors.Capture(err)
	}

	var (
		input     = volumeUUID{UUID: uuid.String()}
		paramsVal volumeRemovalParams
	)

	paramsStmt, err := st.Prepare(`
SELECT &volumeRemovalParams.* FROM (
    SELECT sv.provider_id,
           sv.obliterate_on_cleanup,
           sp.type
    FROM   storage_volume sv
    JOIN   storage_instance_volume siv ON siv.storage_volume_uuid = sv.uuid
    JOIN   storage_instance si ON siv.storage_instance_uuid = si.uuid
    JOIN   storage_pool sp ON si.storage_pool_uuid = sp.uuid
    WHERE  sv.uuid = $volumeUUID.uuid
)
`,
		paramsVal, input,
	)
	if err != nil {
		return storageprovisioning.VolumeRemovalParams{}, errors.Capture(err)
	}

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

		return nil
	})
	if err != nil {
		return storageprovisioning.VolumeRemovalParams{}, errors.Capture(err)
	}

	retVal := storageprovisioning.VolumeRemovalParams{
		Provider:   paramsVal.Type,
		ProviderID: paramsVal.ProviderID,
	}
	if paramsVal.Obliterate.Valid {
		retVal.Obliterate = paramsVal.Obliterate.V
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
