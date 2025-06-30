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

// InitialWatchStatementModelProvisionedFilesystems returns both the namespace
// for watching filesystem life changes where the filesystem is model
// provisioned. On top of this the initial query for getting all filesystems
// in the model that model provisioned is returned.
func (st *State) InitialWatchStatementModelProvisionedFilesystems() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
			SELECT &filesystemID.* FROM storage_filesystem WHERE provision_scope_id=0
		`, filesystemID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var fsIDs []filesystemID
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&fsIDs)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		rval := make([]string, 0, len(fsIDs))
		for _, v := range fsIDs {
			rval = append(rval, v.ID)
		}
		return rval, nil
	}
	return "storage_filesystem_life_model_provisioning", query
}

func (st *State) InitialWatchStatementMachineProvisionedFilesystems(
	netNodeUUID string,
) (string, eventsource.Query[map[string]life.Life]) {
	query := func(
		ctx context.Context,
		db database.TxnRunner,
	) (map[string]life.Life, error) {
		return st.getFilesystemLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_filesystem_life_machine_provisioning", query
}

func (st *State) GetFilesystemLifeForNetNode(
	ctx context.Context,
	netNodeUUID string,
) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getFilesystemLifeForNetNode(ctx, db, netNodeUUID)
}

func (st *State) getFilesystemLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNode{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT (sf.filesystem_id, sf.life_id) AS (&filesystemLife.*)
FROM            storage_filesystem sf
JOIN            storage_filesystem_attachment sfa ON sf.uuid=sfa.storage_filesystem_uuid
WHERE           sf.provision_scope_id=1
AND             sfa.net_node_uuid=$netNode.net_node_uuid
		`, filesystemLife{}, netNode{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var fsLives filesystemLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&fsLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(fsLives.Iter), nil
}

func (st *State) InitialWatchStatementModelProvisionedFilesystemAttachments() (string, eventsource.NamespaceQuery) {
	query := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		stmt, err := st.Prepare(`
SELECT &attachmentUUID.*
FROM   storage_filesystem_attachment
WHERE  provision_scope_id=0
		`, attachmentUUID{})
		if err != nil {
			return nil, errors.Capture(err)
		}
		var fsAttachmentUUIDs []attachmentUUID
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			err := tx.Query(ctx, stmt).GetAll(&fsAttachmentUUIDs)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return nil
		})
		if err != nil {
			return nil, errors.Capture(err)
		}
		rval := make([]string, 0, len(fsAttachmentUUIDs))
		for _, v := range fsAttachmentUUIDs {
			rval = append(rval, v.UUID)
		}
		return rval, nil
	}
	return "storage_filesystem_attachment_life_model_provisioning", query
}

func (st *State) InitialWatchStatementMachineProvisionedFilesystemAttachments(
	netNodeUUID string,
) (string, eventsource.Query[map[string]life.Life]) {
	query := func(ctx context.Context, db database.TxnRunner) (map[string]life.Life, error) {
		return st.getFilesystemAttachmentLifeForNetNode(ctx, db, netNodeUUID)
	}
	return "storage_filesystem_attachment_life_machine_provisioning", query
}

func (st *State) GetFilesystemAttachmentLifeForNetNode(
	ctx context.Context,
	netNodeUUID string,
) (map[string]life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}
	return st.getFilesystemAttachmentLifeForNetNode(ctx, db, netNodeUUID)
}

func (st *State) getFilesystemAttachmentLifeForNetNode(
	ctx context.Context,
	db domain.TxnRunner,
	netNodeUUID string,
) (map[string]life.Life, error) {
	netNodeInput := netNode{UUID: netNodeUUID}
	stmt, err := st.Prepare(`
SELECT DISTINCT &attachmentLife.*
FROM            storage_filesystem_attachment
WHERE           provision_scope_id=1
AND             net_node_uuid=$netNode.net_node_uuid
		`, attachmentLife{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var fsAttachmentLives attachmentLives
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkNetNodeExists(ctx, tx, netNodeUUID)
		if err != nil {
			return err
		} else if !exists {
			return errors.Errorf("net node %q does not exist", netNodeUUID)
		}
		err = tx.Query(ctx, stmt, netNodeInput).GetAll(&fsAttachmentLives)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return maps.Collect(fsAttachmentLives.Iter), nil
}
