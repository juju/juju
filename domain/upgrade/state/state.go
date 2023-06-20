// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
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

func (st *State) getUpgrade(ctx context.Context, tx *sqlair.TX, upgradeUUID string) (info, error) {
	q := `
SELECT * AS &info.* FROM upgrade_info`
	s, err := sqlair.Prepare(q, info{})
	if err != nil {
		return info{}, errors.Annotatef(err, "preparing %q", q)
	}

	var upgradeInfo info
	err = tx.Query(ctx, s).Get(&upgradeInfo)
	if err != nil {
		return info{}, errors.Trace(err)
	}

	return upgradeInfo, nil
}

func (st *State) getUpgradeNodes(ctx context.Context, tx *sqlair.TX, upgradeUUID string) ([]infoControllerNode, error) {
	q := `
SELECT * AS &infoControllerNode.*
FROM   upgrade_info_controller_node
WHERE  upgrade_info_controller_node.upgrade_info_uuid = $M.info_uuid`
	s, err := sqlair.Prepare(q, infoControllerNode{}, sqlair.M{})
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}

	var nodeInfos []infoControllerNode
	err = tx.Query(ctx, s, sqlair.M{"info_uuid": upgradeUUID}).GetAll(&nodeInfos)
	if err != nil && err != sqlair.ErrNoRows {
		return nil, errors.Trace(err)
	}
	return nodeInfos, nil
}

// CreateUpgrade creates an active upgrade to and from specified versions
// and returns the upgrade's UUID
func (st *State) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	upgradeUUID := utils.MustNewUUID().String()
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		q := `
INSERT INTO upgrade_info (uuid, previous_version, target_version, created_at) VALUES
($M.uuid, $M.previous_version, $M.target_version, DATETIME('now'))`
		s, err := sqlair.Prepare(q, sqlair.M{})
		if err != nil {
			return errors.Annotatef(err, "preparing %q", q)
		}
		return errors.Trace(tx.Query(ctx, s, sqlair.M{
			"uuid":             upgradeUUID,
			"previous_version": previousVersion.String(),
			"target_version":   targetVersion.String(),
		}).Run())
	})

	if err != nil {
		return "", errors.Trace(err)
	}
	return upgradeUUID, nil
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

	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		nodeInfos, err := st.getUpgradeNodes(ctx, tx, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		for _, nodeinfo := range nodeInfos {
			if nodeinfo.ControllerNodeID == controllerID {
				return nil
			}
		}
		q := `
INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid) VALUES
($M.uuid, $M.controller_id, $M.info_uuid)`
		s, err := sqlair.Prepare(q, sqlair.M{})
		if err != nil {
			return errors.Annotatef(err, "preparing %q", q)
		}
		return errors.Trace(tx.Query(ctx, s, sqlair.M{
			"uuid":          utils.MustNewUUID().String(),
			"controller_id": controllerID,
			"info_uuid":     upgradeUUID,
		}).Run())
	}))
}

// AllProvisionedControllersReady returns true if and only if all controllers
// that have been started by the provisioner are ready to start the provided upgrade
func (st *State) AllProvisionedControllersReady(ctx context.Context, upgradeUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}
	var (
		provisionedControllers []string
		nodeInfos              []infoControllerNode
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		provisionedControllers, err = st.getProvisionedControllers(ctx, tx)
		if err != nil {
			return errors.Trace(err)
		}
		nodeInfos, err = st.getUpgradeNodes(ctx, tx, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Trace(err)
	}
	ready := set.NewStrings(transform.Slice(nodeInfos, func(info infoControllerNode) string { return info.ControllerNodeID })...)
	missing := set.NewStrings(provisionedControllers...).Difference(ready)
	return missing.IsEmpty(), nil
}

func (st *State) getProvisionedControllers(ctx context.Context, tx *sqlair.TX) ([]string, error) {
	q := `SELECT controller_id FROM controller_node WHERE dqlite_node_id IS NOT NULL`
	s, err := sqlair.Prepare(q)
	if err != nil {
		return nil, errors.Annotatef(err, "preparing %q", q)
	}
	var controllers []string
	err = tx.Query(ctx, s).GetAll(&controllers)
	return controllers, errors.Trace(err)
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
	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		info, err := st.getUpgrade(ctx, tx, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		if info.StartedAt.Valid {
			return nil
		}
		q := `
UPDATE upgrade_info
SET    started_at = DATETIME("now")
WHERE  uuid = $M.uuid`
		s, err := sqlair.Prepare(q, sqlair.M{})
		if err != nil {
			return errors.Annotatef(err, "preparing %q", q)
		}
		return tx.Query(ctx, s, sqlair.M{"uuid": upgradeUUID}).Run()
	}))
}

// SetControllerDone marks the supplied controllerID as having
// completed its upgrades. When SetControllerDone is called by the
// all provisioned controller, the upgrade itself will be completed.
func (st *State) SetControllerDone(ctx context.Context, upgradeUUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		nodeInfos, err := st.getUpgradeNodes(ctx, tx, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		found := false
		for _, nodeInfo := range nodeInfos {
			if nodeInfo.ControllerNodeID == controllerID {
				found = true
				if nodeInfo.NodeUpgradeCompletedAt.Valid {
					return nil
				}
			}
		}
		if !found {
			return errors.Errorf("controller node %q not ready", controllerID)
		}
		err = st.setNodeToDone(ctx, tx, upgradeUUID, controllerID)
		if err != nil {
			return errors.Trace(err)
		}
		err = st.maybeCompleteUpgrade(ctx, tx, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}))
}

func (st *State) setNodeToDone(ctx context.Context, tx *sqlair.TX, infoUUID string, controllerID string) error {
	q := `
UPDATE upgrade_info_controller_node
SET    node_upgrade_completed_at = DATETIME("now")
WHERE  upgrade_info_uuid = $M.info_uuid 
       AND controller_node_id = $M.controller_id`
	s, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", q)
	}
	return errors.Trace(tx.Query(ctx, s, sqlair.M{
		"info_uuid":     infoUUID,
		"controller_id": controllerID,
	}).Run())
}

// TODO (jack-w-shaw) Set `statuses`/`statuseshistory` here
// to status.Available once the table has been added
func (st *State) maybeCompleteUpgrade(ctx context.Context, tx *sqlair.TX, upgradeUUID string) error {
	nodeInfos, err := st.getUpgradeNodes(ctx, tx, upgradeUUID)
	if err != nil {
		return errors.Trace(err)
	}
	for _, nodeInfo := range nodeInfos {
		if !nodeInfo.NodeUpgradeCompletedAt.Valid {
			return nil
		}
	}
	q := `
UPDATE upgrade_info
SET    completed_at = DATETIME("now")
WHERE  uuid = $M.info_uuid`
	s, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", q)
	}
	err = tx.Query(ctx, s, sqlair.M{
		"info_uuid": upgradeUUID,
	}).Run()
	return errors.Trace(err)
}

// ActiveUpgrades returns a slice of the uuids of all active upgrades
func (st *State) ActiveUpgrades(ctx context.Context) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	var activeUpgrades []string
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		q := `
SELECT (uuid) FROM upgrade_info
WHERE  upgrade_info.completed_at IS NULL`
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
