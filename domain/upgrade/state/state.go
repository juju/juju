// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"
	"github.com/juju/version/v2"

	"github.com/juju/juju/domain"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory domain.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CreateUpgrade creates an active upgrade to and from specified versions
// and returns the upgrade's UUID
func (st *State) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	upgradeUUID, err := utils.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	q := "INSERT INTO upgrade_info (uuid, previous_version, target_version, created_at) VALUES (?, ?, ?, DATETIME('now'))"

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q, upgradeUUID.String(), previousVersion.String(), targetVersion.String())
		return errors.Trace(err)
	})

	if err != nil {
		return "", errors.Trace(err)
	}
	return upgradeUUID.String(), nil
}

// SetControllerReady marks the supplied controllerID as being ready
// to start a provided upgrade. All provisioned controllers need to
// be ready before an upgrade can start
// A controller node is ready for an upgrade if a row corresponding
// to the controller is present in upgrade_info_controller_node
func (st *State) SetControllerReady(ctx context.Context, upgradeUUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	lookForReadyNodeQuery := `
SELECT (controller_node_id) AS &infoControllerNode.*
FROM upgrade_info_controller_node
WHERE  upgrade_info_uuid = $M.info_uuid
       AND controller_node_id = $M.controller_id;`
	lookForReadyNodeStatement, err := sqlair.Prepare(lookForReadyNodeQuery, infoControllerNode{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", lookForReadyNodeQuery)
	}

	insertUpgradeNodeQuery := `
INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid)
VALUES
    ($M.uuid, $M.controller_id, $M.info_uuid);`
	insertUpgradeNodeStatement, err := sqlair.Prepare(insertUpgradeNodeQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", insertUpgradeNodeQuery)
	}
	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lookForReadyNodeStatement, sqlair.M{
			"info_uuid":     upgradeUUID,
			"controller_id": controllerID,
		}).Get(&infoControllerNode{})
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}

		err = tx.Query(ctx, insertUpgradeNodeStatement, sqlair.M{
			"uuid":          uuid.String(),
			"controller_id": controllerID,
			"info_uuid":     upgradeUUID,
		}).Run()
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}))
}

// AllProvisionedControllersReady returns true if and only if all controllers
// that have been started by the provisioner are ready to start the provided upgrade
func (st *State) AllProvisionedControllersReady(ctx context.Context, upgradeUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}
	q := `
SELECT COUNT(*)
FROM   controller_node AS node
       LEFT JOIN upgrade_info_controller_node AS upgrade_node
       ON node.controller_id = upgrade_node.controller_node_id
       AND  upgrade_node.upgrade_info_uuid = ?
WHERE  node.dqlite_node_id IS NOT NULL
       AND upgrade_node.controller_node_id IS NULL`

	var allReady bool
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		for rows.Next() {
			var unreadyControllers int
			err := rows.Scan(&unreadyControllers)
			if err != nil {
				return errors.Trace(err)
			}
			allReady = unreadyControllers == 0
		}
		return nil
	})
	if err != nil {
		return false, errors.Trace(err)
	}
	return allReady, nil
}

// StartUpgrade starts the provided upgrade is it exists
//
// TODO (jack-w-shaw) Set `statuses`/`statuseshistory` here
// to status.Busy once the table has been added
func (st *State) StartUpgrade(ctx context.Context, upgradeUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	getUpgradeStartedQuery := "SELECT started_at AS &info.* FROM upgrade_info WHERE uuid = $M.info_uuid"
	getUpgradeStartedStatement, err := sqlair.Prepare(getUpgradeStartedQuery, info{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", getUpgradeStartedQuery)
	}

	startUpgradeQuery := "UPDATE upgrade_info SET started_at = DATETIME('now') WHERE uuid = $M.info_uuid"
	startUpgradeStatement, err := sqlair.Prepare(startUpgradeQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", startUpgradeQuery)
	}

	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var node info
		err := tx.Query(ctx, getUpgradeStartedStatement, sqlair.M{"info_uuid": upgradeUUID}).Get(&node)
		if err != nil {
			return errors.Trace(err)
		}
		// We use the presence of StartedAt as a flag to indicate whether the upgrade has started.
		// It's specific value is only for debugging
		if node.StartedAt.Valid {
			return nil
		}
		err = tx.Query(ctx, startUpgradeStatement, sqlair.M{"info_uuid": upgradeUUID}).Run()
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}))
}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// all provisioned controller, the upgrade itself will be completed.
//
// TODO (jack-w-shaw) Set `statuses`/`statuseshistory` here
// to status.Available when we complete an upgrade
func (st *State) SetControllerDone(ctx context.Context, upgradeUUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	lookForDoneNodesQuery := `
SELECT (controller_node_id, node_upgrade_completed_at) AS &infoControllerNode.*
FROM   upgrade_info_controller_node
WHERE  upgrade_info_uuid = $M.info_uuid`
	lookForDoneNodesStatement, err := sqlair.Prepare(lookForDoneNodesQuery, infoControllerNode{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", lookForDoneNodesQuery)
	}

	setNodeToDoneQuery := `
UPDATE upgrade_info_controller_node
SET    node_upgrade_completed_at = DATETIME("now")
WHERE  upgrade_info_uuid = $M.info_uuid 
       AND controller_node_id = $M.controller_id`
	setNodeToDoneStatement, err := sqlair.Prepare(setNodeToDoneQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", setNodeToDoneQuery)
	}

	completeUpgradeQuery := `
UPDATE upgrade_info
SET    completed_at = DATETIME("now")
WHERE  uuid = $M.info_uuid`
	completeUpgradeStatement, err := sqlair.Prepare(completeUpgradeQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", completeUpgradeQuery)
	}

	var (
		controllerReady bool
	)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var nodes []infoControllerNode
		err := tx.Query(ctx, lookForDoneNodesStatement, sqlair.M{
			"info_uuid": upgradeUUID,
		}).GetAll(&nodes)
		if err != nil && err != sql.ErrNoRows {
			return errors.Trace(err)
		}

		for _, node := range nodes {
			if node.ControllerNodeID == controllerID {
				controllerReady = true
				// We use the presence of NodeUpgradeCompletedAt as a flag to indicate whether the
				// upgrade has been completed.  It's specific value is only for debugging
				if node.NodeUpgradeCompletedAt.Valid {
					return nil
				}
			}
		}
		if !controllerReady {
			return nil
		}

		err = tx.Query(ctx, setNodeToDoneStatement, sqlair.M{
			"info_uuid":     upgradeUUID,
			"controller_id": controllerID,
		}).Run()
		if err != nil {
			return errors.Trace(err)
		}
		for _, node := range nodes {
			if !node.NodeUpgradeCompletedAt.Valid && node.ControllerNodeID != controllerID {
				return nil
			}
		}

		err = tx.Query(ctx, completeUpgradeStatement, sqlair.M{"info_uuid": upgradeUUID}).Run()
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	if !controllerReady {
		return errors.Errorf("controller node %q not ready", controllerID)
	}
	return nil
}

// ActiveUpgrades returns a slice of the uuids of all active upgrades
func (st *State) ActiveUpgrades(ctx context.Context) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var activeUpgrades []string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := "SELECT (uuid) FROM upgrade_info WHERE completed_at IS NULL"
		rows, err := tx.QueryContext(ctx, q)
		if err != nil && err != sql.ErrNoRows {
			return errors.Trace(err)
		}
		defer func() { _ = rows.Close() }()

		for rows.Next() {
			var uuid string
			if err := rows.Scan(&uuid); err != nil {
				return errors.Trace(err)
			}
			activeUpgrades = append(activeUpgrades, uuid)
		}
		return rows.Err()
	})
	return activeUpgrades, errors.Trace(err)
}
