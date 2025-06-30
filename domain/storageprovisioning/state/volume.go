// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"maps"

	"github.com/canonical/sqlair"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

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

func (st *State) InitialWatchStatementMachineProvisionedVolumes(netNodeUUID string) (string, eventsource.Query[map[string]life.Life]) {
	query := func(
		ctx context.Context,
		db database.TxnRunner,
	) (map[string]life.Life, error) {
		return st.getVolumeLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_life_machine_provisioning", query
}

func (st *State) GetVolumeLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getVolumeLifeForNetNode(ctx, db, netNodeUUID)
}

func (st *State) getVolumeLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNode{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sv.volume_id, sv.life_id) AS (&volumeLife.*)
FROM            storage_volume sv
JOIN            storage_volume_attachment sva ON sv.uuid=sva.storage_volume_uuid
WHERE           sv.provision_scope_id=1
AND             sva.net_node_uuid=$netNode.net_node_uuid
		`, volumeLife{})
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

func (st *State) InitialWatchStatementMachineProvisionedVolumeAttachments(netNodeUUID string) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]life.Life, error) {
		return st.getVolumeAttachmentLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_attachment_life_machine_provisioning", query
}

func (st *State) GetVolumeAttachmentLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getVolumeAttachmentLifeForNetNode(ctx, db, netNodeUUID)
}

func (st *State) getVolumeAttachmentLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNode{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_volume_attachment
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNode.net_node_uuid
		`, attachmentLife{})
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

func (st *State) InitialWatchStatementVolumeAttachmentPlans(netNodeUUID string) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]life.Life, error) {
		return st.getVolumeAttachmentPlanLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_volume_attachment_plan_life_machine_provisioning", query
}

func (st *State) GetVolumeAttachmentPlanLifeForNetNode(ctx context.Context, netNodeUUID string) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getVolumeAttachmentPlanLifeForNetNode(ctx, db, netNodeUUID)
}

func (st *State) getVolumeAttachmentPlanLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNode{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_volume_attachment_plan
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNode.net_node_uuid
		`, attachmentLife{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var volAttachmentPlanLives attachmentLives
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
