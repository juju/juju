// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/upgrade"
	"github.com/juju/juju/domain"
	domainupgrade "github.com/juju/juju/domain/upgrade"
	upgradeerrors "github.com/juju/juju/domain/upgrade/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/uuid"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
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
	info := Info{
		UUID:            upgradeUUID.String(),
		PreviousVersion: previousVersion.String(),
		TargetVersion:   targetVersion.String(),
		StateIDType:     int(upgrade.Created),
	}

	stmt, err := st.Prepare(`
INSERT INTO upgrade_info (*) 
VALUES ($Info.*)`, info)
	if err != nil {
		return "", errors.Annotatef(err, "preparing insert upgrade info statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, info).Run()
		if database.IsErrConstraintUnique(err) {
			return upgradeerrors.AlreadyExists
		} else if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return "", errors.Trace(err)
	}

	return domainupgrade.UUID(upgradeUUID.String()), nil
}

// SetControllerReady marks the supplied controllerID as being ready
// to start a provided upgrade. All provisioned controllers need to
// be ready before an upgrade can start.
// A controller node is ready for an upgrade if a row corresponding
// to the controller is present in upgrade_info_controller_node.
func (st *State) SetControllerReady(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	uuid, err := uuid.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}

	controllerNodeInfo := ControllerNodeInfo{
		UUID:             uuid.String(),
		UpgradeInfoUUID:  upgradeUUID.String(),
		ControllerNodeID: controllerID,
	}

	checkExistsNodeStmt, err := st.Prepare(`
SELECT  &ControllerNodeInfo.controller_node_id
FROM    upgrade_info_controller_node
WHERE   upgrade_info_uuid = $ControllerNodeInfo.upgrade_info_uuid
AND     controller_node_id = $ControllerNodeInfo.controller_node_id;
`, controllerNodeInfo)
	if err != nil {
		return errors.Annotatef(err, "preparing check exists node statement")
	}

	insertUpgradeNodeStmt, err := st.Prepare(`
INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid)
VALUES ($ControllerNodeInfo.*);
`, controllerNodeInfo)
	if err != nil {
		return errors.Annotatef(err, "preparing insert upgrade node statement")
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, checkExistsNodeStmt, controllerNodeInfo).Get(&ControllerNodeInfo{})
		if err == nil {
			// The controller node already exists, so return.
			return nil
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}

		err = tx.Query(ctx, insertUpgradeNodeStmt, controllerNodeInfo).Run()
		if database.IsErrConstraintForeignKey(err) {
			return errors.Annotatef(upgradeerrors.NotFound, "upgrade %q", upgradeUUID)
		} else if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	return errors.Trace(err)
}

// AllProvisionedControllersReady returns true if and only if all controllers
// that have been started by the provisioner are ready to start the provided
// upgrade.
func (st *State) AllProvisionedControllersReady(ctx context.Context, upgradeUUID domainupgrade.UUID) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}
	var count Count
	controllerNodeInfo := ControllerNodeInfo{
		UpgradeInfoUUID: upgradeUUID.String(),
	}
	stmt, err := st.Prepare(`
SELECT COUNT(*) AS &Count.num
FROM   controller_node AS node
       LEFT JOIN upgrade_info_controller_node AS upgrade_node
       ON node.controller_id = upgrade_node.controller_node_id
       AND  upgrade_node.upgrade_info_uuid = $ControllerNodeInfo.upgrade_info_uuid
WHERE  node.dqlite_node_id IS NOT NULL
AND    upgrade_node.controller_node_id IS NULL;
`, count, controllerNodeInfo)
	if err != nil {
		return false, errors.Annotate(err, "preparing select count provisioned controllers statement")
	}

	var allReady bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, controllerNodeInfo).Get(&count)
		if err != nil {
			return errors.Trace(err)
		}
		allReady = count.Num == 0
		return nil
	})
	if err != nil {
		return false, errors.Trace(err)
	}
	return allReady, nil
}

// StartUpgrade starts the provided upgrade if the upgrade already exists. If it
// does not exists it returns a NotFound error. If it's already started, it
// returns a AlreadyStarted error.
//
// TODO (jack-w-shaw) Set `statuses`/`statuseshistory` here
// to status.Busy once the table has been added
func (st *State) StartUpgrade(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	info := Info{
		UUID: upgradeUUID.String(),
	}

	getUpgradeStartedStmt, err := st.Prepare(`
SELECT &Info.state_type_id 
FROM   upgrade_info 
WHERE  uuid = $Info.uuid
`, info)
	if err != nil {
		return errors.Annotate(err, "preparing select started upgrade statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, getUpgradeStartedStmt, info).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			if database.IsErrNotFound(err) {
				return errors.Annotatef(upgradeerrors.NotFound, "upgrade %q", upgradeUUID)
			}
		}
		if err != nil {
			return errors.Trace(err)
		}

		// If the upgrade is already started, return an error.
		if err := upgrade.State(info.StateIDType).TransitionTo(upgrade.Started); err != nil {
			if errors.Is(err, upgrade.ErrAlreadyAtState) {
				return errors.Annotatef(upgradeerrors.AlreadyStarted, "upgrade %q already started", upgradeUUID)
			}
			return errors.Trace(err)
		}

		// Start the upgrade by setting the state to "Started".
		err = st.updateState(ctx, tx, info.UUID, upgrade.Created, upgrade.Started)
		if err != nil {
			return errors.Annotatef(err, "expected to set upgrade state to started")
		}
		return nil
	})
	return errors.Trace(err)
}

// SetDBUpgradeCompleted marks the database upgrade as completed.
func (st *State) SetDBUpgradeCompleted(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.updateState(ctx, tx, upgradeUUID.String(), upgrade.Started, upgrade.DBCompleted)
		if err != nil {
			return errors.Annotatef(err, "expected to set upgrade state to db complete")
		}
		return nil
	})
	return errors.Trace(err)
}

// SetDBUpgradeFailed marks the database upgrade as failed.
func (st *State) SetDBUpgradeFailed(ctx context.Context, upgradeUUID domainupgrade.UUID) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.updateState(ctx, tx, upgradeUUID.String(), upgrade.Started, upgrade.Error)
		if err != nil {
			return errors.Annotatef(err, "expected to set upgrade state to error")
		}
		return nil
	})
	return errors.Trace(err)
}

// SetControllerDone marks the supplied controllerID as having completed its
// upgrades. When SetControllerDone is called by the all provisioned controller,
// the upgrade itself will be completed.
//
// TODO (jack-w-shaw) Set `statuses`/`statuseshistory` here to status.Available
// when we complete an upgrade
func (st *State) SetControllerDone(ctx context.Context, upgradeUUID domainupgrade.UUID, controllerID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	controllerNodeInfo := ControllerNodeInfo{
		UpgradeInfoUUID:  upgradeUUID.String(),
		ControllerNodeID: controllerID,
	}
	info := Info{
		UUID: upgradeUUID.String(),
	}

	lookForDoneNodesStmt, err := st.Prepare(`
SELECT (controller_node_id, node_upgrade_completed_at) AS (&ControllerNodeInfo.*)
FROM   upgrade_info_controller_node
WHERE  upgrade_info_uuid = $ControllerNodeInfo.upgrade_info_uuid
AND    controller_node_id = $ControllerNodeInfo.controller_node_id;
`, controllerNodeInfo)
	if err != nil {
		return errors.Annotate(err, "preparing select done query")
	}

	setNodeToDoneStmt, err := st.Prepare(`
UPDATE  upgrade_info_controller_node
SET     node_upgrade_completed_at = DATETIME("now")
WHERE   upgrade_info_uuid = $ControllerNodeInfo.upgrade_info_uuid
AND     controller_node_id = $ControllerNodeInfo.controller_node_id
AND     node_upgrade_completed_at IS NULL;
`, controllerNodeInfo)
	if err != nil {
		return errors.Annotatef(err, "preparing update node query")
	}

	m := sqlair.M{
		"from_state": upgrade.DBCompleted,
		"to_state":   upgrade.StepsCompleted,
	}
	completeUpgradeStmt, err := st.Prepare(`
UPDATE upgrade_info
SET    state_type_id = $M.to_state
WHERE  uuid = $Info.uuid AND state_type_id = $M.from_state
AND (
    SELECT COUNT(*)
	FROM   upgrade_info_controller_node
    WHERE  upgrade_info_uuid = $Info.uuid
    AND    node_upgrade_completed_at IS NOT NULL
) = (
    SELECT COUNT(*) 
    FROM   upgrade_info_controller_node
    WHERE  upgrade_info_uuid = $Info.uuid
);
`, info, m)
	if err != nil {
		return errors.Annotatef(err, "preparing complete upgrade query")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var node ControllerNodeInfo
		err := tx.Query(ctx, lookForDoneNodesStmt, controllerNodeInfo).Get(&node)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("controller node %q not ready", controllerID)
			}
			return errors.Trace(err)
		}

		err = tx.Query(ctx, setNodeToDoneStmt, controllerNodeInfo).Run()
		if err != nil {
			return errors.Trace(err)
		}

		return errors.Trace(tx.Query(ctx, completeUpgradeStmt, info, m).Run())
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ActiveUpgrade returns the uuid of the active upgrade. The active upgrade is
// any upgrade that is not in the StepsCompleted state. It returns a NotFound
// error if there is no active upgrade.
func (st *State) ActiveUpgrade(ctx context.Context) (domainupgrade.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	info := Info{
		StateIDType: int(upgrade.StepsCompleted),
	}

	stmt, err := st.Prepare(`
SELECT &Info.uuid
FROM upgrade_info 
WHERE state_type_id < $Info.state_type_id
`, info)
	if err != nil {
		return "", errors.Annotate(err, "preparing select active upgrade statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, info).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(upgradeerrors.NotFound, "active upgrade")
		} else if err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	return domainupgrade.UUID(info.UUID), errors.Trace(err)
}

// UpgradeInfo returns the upgrade info for the provided upgradeUUID. It returns
// a NotFound error if the upgrade does not exist.
func (st *State) UpgradeInfo(ctx context.Context, upgradeUUID domainupgrade.UUID) (upgrade.Info, error) {
	db, err := st.DB()
	if err != nil {
		return upgrade.Info{}, errors.Trace(err)
	}
	info := Info{
		UUID: upgradeUUID.String(),
	}

	stmt, err := st.Prepare(`
SELECT &Info.*
FROM upgrade_info 
WHERE uuid = $Info.uuid
`, info)

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, info).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(upgradeerrors.NotFound, "upgrade %q", upgradeUUID)
		}
		return err
	})
	if err != nil {
		return upgrade.Info{}, errors.Trace(err)
	}

	result, err := info.ToUpgradeInfo()
	return result, errors.Trace(err)
}

// updateState updates the state of an ongoing upgrade.
func (st *State) updateState(ctx context.Context, tx *sqlair.TX, uuid string, from upgrade.State, to upgrade.State) error {
	if err := from.TransitionTo(to); err != nil {
		if errors.Is(err, upgrade.ErrAlreadyAtState) {
			return nil
		}
		return errors.Trace(err)
	}

	info := Info{
		UUID: uuid,
	}
	m := sqlair.M{
		"from": from,
		"to":   to,
	}
	stmt, err := st.Prepare(`
UPDATE upgrade_info 
SET state_type_id = $M.to
WHERE uuid = $Info.uuid
AND state_type_id = $M.from;`, info, m)
	if err != nil {
		return errors.Annotatef(err, "preparing update from %q to %q statement", from, to)
	}

	var outcome sqlair.Outcome
	if err = tx.Query(ctx, stmt, info, m).Get(&outcome); err != nil {
		return errors.Trace(err)
	}
	if num, err := outcome.Result().RowsAffected(); err != nil {
		return errors.Trace(err)
	} else if num != 1 {
		return errors.Errorf("setting from %q to %q, but %d rows were affected", from, to, num)
	}
	return nil
}

func (*State) NamespaceForWatchUpgradeReady() string {
	return "upgrade_info_controller_node"
}
func (*State) NamespaceForWatchUpgradeState() string {
	return "upgrade_info"
}
