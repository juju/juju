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
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/errors"
)

// checkVolumeExists checks if a volume for the provided uuid exists.
// Returning when this case is satisfied.
func (st *State) checkVolumeExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid domainstorageprovisioning.VolumeUUID,
) (bool, error) {
	volumeUUIDInput := volumeUUID{UUID: uuid.String()}

	checkQuery, err := st.Prepare(`
SELECT &volumeUUID.*
FROM   storage_volume
WHERE  uuid = $volumeUUID.uuid
`,
		volumeUUIDInput,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, checkQuery, volumeUUIDInput).Get(&volumeUUIDInput)
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
) (map[string]domainstorageprovisioning.VolumeAttachmentID, error) {
	if len(uuids) == 0 {
		return nil, nil
	}

	db, err := st.DB()
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

	rval := make(map[string]domainstorageprovisioning.VolumeAttachmentID, len(dbVals))
	for _, v := range dbVals {
		id := domainstorageprovisioning.VolumeAttachmentID{
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
	uuid domainstorageprovisioning.VolumeAttachmentUUID,
) (domainlife.Life, error) {
	db, err := st.DB()
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
	db, err := st.DB()
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
	db, err := st.DB()
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
	vUUID domainstorageprovisioning.VolumeUUID,
	nodeUUID domainnetwork.NetNodeUUID,
) (domainstorageprovisioning.VolumeAttachmentUUID, error) {
	db, err := st.DB()
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

	return domainstorageprovisioning.VolumeAttachmentUUID(dbVal.UUID), nil
}

// GetVolumeLife returns the current life value for a
// volume uuid.
//
// The following errors may be returned:
// - [storageprovisioningerrors.VolumeNotFound] when no volume exists for the
// provided uuid.
func (st *State) GetVolumeLife(
	ctx context.Context,
	uuid domainstorageprovisioning.VolumeUUID,
) (domainlife.Life, error) {
	db, err := st.DB()
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
	db, err := st.DB()
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
) (domainstorageprovisioning.VolumeUUID, error) {
	db, err := st.DB()
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

	return domainstorageprovisioning.VolumeUUID(dbVal.UUID), nil
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
