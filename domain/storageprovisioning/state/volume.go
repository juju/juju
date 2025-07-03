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
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/errors"
)

// GetVolumeAttachmentIDs returns the [storageprovisioning.VolumeAttachmentID]
// information for each volume attachment uuid supplied. If a uuid does not
// exist or isn't attached to either a machine or a unit then it will not exist
// in the result.
func (st *State) GetVolumeAttachmentIDs(
	ctx context.Context, uuids []string,
) (map[string]storageprovisioning.VolumeAttachmentID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	uuidInputs := volumeAttachmentUUIDs(uuids)

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
	LEFT JOIN   machine m ON sva.net_node_uuid != m.net_node_uuid
	JOIN        unit u ON sva.net_node_uuid = u.net_node_uuid
	WHERE       sva.uuid IN ($volumeAttachmentUUIDs[:])
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

// GetVolumeAttachmentLifeForNetNode returns a mapping of volume
// attachment uuid to the current life value for each machine provisioned
// volume attachment that is to be provisioned by the machine owning the
// supplied net node.
func (st *State) GetVolumeAttachmentLifeForNetNode(
	ctx context.Context, netNodeUUID string,
) (map[string]life.Life, error) {
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
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNodeUUIDRef{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_volume_attachment
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNodeUUIDRef.net_node_uuid
		`, attachmentLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var volAttachmentLives attachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
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
	ctx context.Context, netNodeUUID string,
) (map[string]life.Life, error) {
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
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNodeUUIDRef{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sv.volume_id, svap.life_id) AS (&volumeAttachmentPlanLife.*)
FROM            storage_volume_attachment_plan svap
JOIN            storage_volume sv ON svap.storage_volume_uuid=sv.uuid
WHERE           svap.provision_scope_id=1
AND             svap.net_node_uuid=$netNodeUUIDRef.net_node_uuid
		`, volumeAttachmentPlanLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var volAttachmentPlanLives volumeAttachmentPlanLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
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

// GetVolumeLifeForNetNode returns a mapping of volume id to current life value
// for each machine provisioned volume that is to be provisioned by the machine
// owning the supplied net node.
func (st *State) GetVolumeLifeForNetNode(
	ctx context.Context, netNodeUUID string,
) (map[string]life.Life, error) {
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
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNodeUUIDRef{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sv.volume_id, sv.life_id) AS (&volumeLife.*)
FROM            storage_volume sv
JOIN            storage_volume_attachment sva ON sv.uuid=sva.storage_volume_uuid
WHERE           sv.provision_scope_id=1
AND             sva.net_node_uuid=$netNodeUUIDRef.net_node_uuid
		`, volumeLife{}, netNodeInput)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var volLives volumeLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
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

// InitialWatchStatementMachineProvisionedVolumes returns both the namespace for
// watching volume life changes where the volume is machine provisioned. On top
// of this the initial query for getting all volumes in the model that are
// machine provisioned is returned.
//
// Only volumes that can be provisioned by the machine connected to the
// supplied net node will be emitted.
func (st *State) InitialWatchStatementMachineProvisionedVolumes(netNodeUUID string) (string, eventsource.Query[map[string]life.Life]) {
	query := func(
		ctx context.Context,
		db database.TxnRunner,
	) (map[string]life.Life, error) {
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
func (st *State) InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID string) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]life.Life, error) {
		return st.getVolumeAttachmentLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_attachment_life_machine_provisioning", query
}

// InitialWatchStatementModelProvisionedVolumeAttachments returns both the
// namespace for watching volume attachment life changes where the volume
// attachment is model provisioned. On top of this the initial query for getting
// all volume attachments in the model that are model provisioned is returned.
func (st *State) InitialWatchStatementModelProvisionedVolumeAttachments() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT &attachmentUUID.*
FROM   storage_volume_attachment
WHERE  provision_scope_id=0
		`, attachmentUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var volAttachmentUUIDs []attachmentUUID
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
func (st *State) InitialWatchStatementVolumeAttachmentPlans(netNodeUUID string) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]life.Life, error) {
		return st.getVolumeAttachmentPlanLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_attachment_plan_life_machine_provisioning", query
}
