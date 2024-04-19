// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/domain"
	. "github.com/juju/juju/domain/query"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/uuid"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CreateUpgrade creates an active upgrade to and from specified versions
// and returns the upgrade's UUID. If an active upgrade already exists,
// return an AlreadyExists error
func (st *State) CreateUpgrade(ctx context.Context, previousVersion, targetVersion version.Number) (domainupgrade.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	upgradeUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Trace(err)
	}
	q := "INSERT INTO upgrade_info (uuid, previous_version, target_version, state_type_id) VALUES (?, ?, ?, ?)"

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, q,
			upgradeUUID.String(),
			previousVersion.String(),
			targetVersion.String(),
			upgrade.Created,
		)
		return errors.Trace(err)
	})

	if err != nil {
		return "", errors.Trace(err)
	}
	return domainupgrade.UUID(upgradeUUID.String()), nil
}

// SetControllerReady marks the supplied controllerID as being ready
// to start a provided upgrade. All provisioned controllers need to
// be ready before an upgrade can start
// A controller node is ready for an upgrade if a row corresponding
// to the controller is present in upgrade_info_controller_node
func (st *State) SetControllerReady(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	lookForReadyNodeQuery := `
SELECT  controller_node_id AS &infoControllerNode.controller_node_id
FROM    upgrade_info_controller_node
WHERE   upgrade_info_uuid = $M.info_uuid
AND     controller_node_id = $M.controller_id;
`
	lookForReadyNodeStmt, err := st.Prepare(lookForReadyNodeQuery, infoControllerNode{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", lookForReadyNodeQuery)
	}

	insertUpgradeNodeQuery := `
INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid)
VALUES ($M.uuid, $M.controller_id, $M.info_uuid);
`
	insertUpgradeNodeStmt, err := st.Prepare(insertUpgradeNodeQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", insertUpgradeNodeQuery)
	}
	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, lookForReadyNodeStmt, sqlair.M{
			"info_uuid":     upgradeUUID,
			"controller_id": controllerID,
		}).Get(&infoControllerNode{})
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}

		err = tx.Query(ctx, insertUpgradeNodeStmt, sqlair.M{
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
// that have been started by the provisioner are ready to start the provided
// upgrade
func (st *State) AllProvisionedControllersReady(ctx context.Context, upgradeUUID domainupgrade.UUID) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}
	q := `
SELECT COUNT(*)
FROM    controller_node AS node
        LEFT JOIN upgrade_info_controller_node AS upgrade_node
            ON node.controller_id = upgrade_node.controller_node_id
            AND  upgrade_node.upgrade_info_uuid = ?
WHERE  node.dqlite_node_id IS NOT NULL
AND    upgrade_node.controller_node_id IS NULL;
`

	var allReady bool
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, q, upgradeUUID)
		if err != nil {
			return errors.Trace(err)
		}
		defer rows.Close()

		for rows.Next() {
			var unreadyControllers int
			err = rows.Scan(&unreadyControllers)
			if err != nil {
				return errors.Trace(err)
			}
			allReady = unreadyControllers == 0
		}

		return rows.Err()
	})
	if err != nil {
		return false, errors.Trace(err)
	}
	return allReady, nil
}

// StartUpgrade starts the provided upgrade if the upgrade already exists.
// If it's already started, it becomes a no-op.
//
// TODO (jack-w-shaw) Set `statuses`/`statuseshistory` here
// to status.Busy once the table has been added
func (st *State) StartUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	getUpgradeStartedQuery := `
SELECT upgrade_state_type.id 
FROM upgrade_info 
	LEFT JOIN upgrade_state_type
		ON upgrade_info.state_type_id = upgrade_state_type.id
WHERE uuid = ?
`

	startUpgradeQuery := "UPDATE upgrade_info SET state_type_id = ? WHERE uuid = ? AND state_type_id = ?"

	return errors.Trace(db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var state int
		row := tx.QueryRowContext(ctx, getUpgradeStartedQuery, upgradeUUID)
		if err := row.Scan(&state); err != nil {
			return errors.Trace(err)
		}

		// If the upgrade is already started, we don't need to do anything.
		if err := upgrade.State(state).TransitionTo(upgrade.Started); err != nil {
			if errors.Is(err, upgrade.ErrAlreadyAtState) {
				return errors.Annotatef(upgradeerrors.ErrUpgradeAlreadyStarted, "upgrade %q already started", upgradeUUID)
			}
			return errors.Trace(err)
		}

		// Start the upgrade by setting the state to Started.
		result, err := tx.ExecContext(ctx, startUpgradeQuery, upgrade.Started, upgradeUUID, upgrade.Created)
		if err != nil {
			return errors.Trace(err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if num := affected; num != 1 {
			return errors.Annotatef(upgradeerrors.ErrUpgradeAlreadyStarted, "expected to start upgrade, but %d rows were affected", num)
		}
		return nil
	}))
}

// SetDBUpgradeCompleted marks the database upgrade as completed
func (st *State) SetDBUpgradeCompleted(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
UPDATE upgrade_info 
SET state_type_id = $M.to_state 
WHERE uuid = $M.info_uuid
AND state_type_id = $M.from_state;`

	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		in := sqlair.M{
			"info_uuid":  upgradeUUID,
			"from_state": upgrade.Started,
			"to_state":   upgrade.DBCompleted,
		}
		res, err := st.Exec(ctx, tx, q, In(in))
		if err != nil {
			return errors.Trace(err)
		}
		if num, err := res.RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return errors.Errorf("expected to set db upgrade completed, but %d rows were affected", num)
		}
		return nil
	}))
}

// SetDBUpgradeFailed marks the database upgrade as failed
func (st *State) SetDBUpgradeFailed(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
UPDATE upgrade_info 
SET state_type_id = $M.to_state 
WHERE uuid = $M.info_uuid
AND state_type_id = $M.from_state;`
	completedDBUpgradeStmt, err := st.Prepare(q, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing %q", q)
	}

	return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var outcome sqlair.Outcome
		if err = tx.Query(ctx, completedDBUpgradeStmt, sqlair.M{
			"info_uuid":  upgradeUUID,
			"from_state": upgrade.Started,
			"to_state":   upgrade.Error,
		}).Get(&outcome); err != nil {
			return errors.Trace(err)
		}
		if num, err := outcome.Result().RowsAffected(); err != nil {
			return errors.Trace(err)
		} else if num != 1 {
			return errors.Errorf("expected to set db upgrade failed, but %d rows were affected", num)
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
func (st *State) SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	lookForDoneNodesQuery := `
SELECT (controller_node_id, node_upgrade_completed_at) AS (&infoControllerNode.*)
FROM   upgrade_info_controller_node
WHERE  upgrade_info_uuid = $M.info_uuid
AND    controller_node_id = $M.controller_id;`
	lookForDoneNodesStmt, err := st.Prepare(lookForDoneNodesQuery, infoControllerNode{}, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing select done query")
	}

	setNodeToDoneQuery := `
UPDATE  upgrade_info_controller_node
SET     node_upgrade_completed_at = DATETIME("now")
WHERE   upgrade_info_uuid = $M.info_uuid
AND     controller_node_id = $M.controller_id
AND     node_upgrade_completed_at IS NULL;
`
	setNodeToDoneStmt, err := st.Prepare(setNodeToDoneQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing update node query")
	}

	completeUpgradeQuery := `
UPDATE upgrade_info
SET    state_type_id = $M.to_state
WHERE  uuid = $M.info_uuid AND state_type_id = $M.from_state
AND (
    SELECT COUNT(*)
	FROM   upgrade_info_controller_node
    WHERE  upgrade_info_uuid = $M.info_uuid
    AND    node_upgrade_completed_at IS NOT NULL
) = (
    SELECT COUNT(*) 
	FROM   upgrade_info_controller_node
    WHERE  upgrade_info_uuid = $M.info_uuid
);
`
	completeUpgradeStmt, err := st.Prepare(completeUpgradeQuery, sqlair.M{})
	if err != nil {
		return errors.Annotatef(err, "preparing complete upgrade query")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var node infoControllerNode
		err := tx.Query(ctx, lookForDoneNodesStmt, sqlair.M{
			"info_uuid":     upgradeUUID,
			"controller_id": controllerID,
		}).Get(&node)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("controller node %q not ready", controllerID)
			}
			return errors.Trace(err)
		}

		err = tx.Query(ctx, setNodeToDoneStmt, sqlair.M{
			"info_uuid":     upgradeUUID,
			"controller_id": controllerID,
		}).Run()
		if err != nil {
			return errors.Trace(err)
		}

		var outcome sqlair.Outcome
		err = tx.Query(ctx, completeUpgradeStmt, sqlair.M{
			"info_uuid":  upgradeUUID,
			"from_state": upgrade.DBCompleted,
			"to_state":   upgrade.StepsCompleted,
		}).Get(&outcome)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ActiveUpgrade returns the uuid of the active upgrade.
// The active upgrade is any upgrade that is not in the StepsCompleted state.
func (st *State) ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	var activeUpgrade domainupgrade.UUID
	q := "SELECT (uuid) FROM upgrade_info WHERE state_type_id < ?"

	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, q, upgrade.StepsCompleted)
		if err := row.Scan(&activeUpgrade); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	return activeUpgrade, errors.Trace(err)
}

// UpgradeInfo returns the upgrade info for the provided upgradeUUID
func (st *State) UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error) {
	db, err := st.DB()
	if err != nil {
		return upgrade.Info{}, errors.Trace(err)
	}

	q := `
SELECT uuid, previous_version, target_version, upgrade_state_type.id 
FROM upgrade_info 
	LEFT JOIN upgrade_state_type
		ON upgrade_info.state_type_id = upgrade_state_type.id
WHERE uuid = ?
	`

	var upgradeInfoRow info
	err = db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, q, upgradeUUID)
		if err := row.Scan(&upgradeInfoRow.UUID, &upgradeInfoRow.PreviousVersion, &upgradeInfoRow.TargetVersion, &upgradeInfoRow.State); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return upgrade.Info{}, errors.Trace(err)
	}

	result, err := upgradeInfoRow.ToUpgradeInfo()
	return result, errors.Trace(err)
}
