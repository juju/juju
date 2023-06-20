// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

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

var noUpgradeErr = errors.Errorf("no current upgrade")

// NewState creates a state to access the database.
func NewState(factory domain.DBFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// EnsureUpgradeInfo returns the current upgrade info and ensures the
// current controller is included and made ready. If no upgrade exists,
// one is created
func (st *State) EnsureUpgradeInfo(ctx context.Context, controllerID string, previousVersion, targetVersion version.Number) (Info, []InfoControllerNode, error) {
	db, err := st.DB()
	if err != nil {
		return Info{}, nil, errors.Trace(err)
	}

	var (
		resInfo      Info
		resNodeInfos []InfoControllerNode
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		resInfo, resNodeInfos, err = st.getCurrentUpgrade(ctx, tx)
		if err != nil && err != noUpgradeErr {
			return errors.Trace(err)
		}
		if err == noUpgradeErr {
			resInfo, resNodeInfos, err = st.startUpgrade(ctx, tx, controllerID, previousVersion, targetVersion)
			return errors.Trace(err)
		}
		if err := verifyVersionMatches(resInfo, previousVersion, targetVersion); err != nil {
			return errors.Trace(err)
		}
		resInfo, resNodeInfos, err = st.ensureNodeReady(ctx, tx, resInfo, resNodeInfos, controllerID)
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(err)
	})
	return resInfo, resNodeInfos, errors.Trace(err)
}

func (st *State) getCurrentUpgrade(ctx context.Context, tx *sqlair.TX) (Info, []InfoControllerNode, error) {
	q1 := `
SELECT (uuid, previous_version, target_version, init_time) AS &Info.* FROM upgrade_info AS info
WHERE info.completion_time IS NULL
ORDER BY info.init_time DESC LIMIT 1`
	s1, err := sqlair.Prepare(q1, Info{})
	if err != nil {
		return Info{}, nil, errors.Annotatef(err, "preparing %q", q1)
	}

	q2 := `
SELECT (controller_node_id, status) AS &InfoControllerNode.*
FROM   upgrade_info_controller_node
       JOIN upgrade_node_status AS status
       ON upgrade_info_controller_node.upgrade_node_status_id = status.id
WHERE upgrade_info_controller_node.upgrade_info_uuid = $M.info_uuid `
	s2, err := sqlair.Prepare(q2, InfoControllerNode{}, sqlair.M{})
	if err != nil {
		return Info{}, nil, errors.Annotatef(err, "preparing %q", q2)
	}

	var info Info
	err = tx.Query(ctx, s1).Get(&info)
	if err == sqlair.ErrNoRows {
		return Info{}, nil, noUpgradeErr
	}
	if err != nil {
		return Info{}, nil, errors.Trace(err)
	}

	var nodeInfos []InfoControllerNode
	err = tx.Query(ctx, s2, sqlair.M{"info_uuid": info.UUID}).GetAll(&nodeInfos)
	if err != nil && err != sqlair.ErrNoRows {
		return Info{}, nil, errors.Trace(err)
	}
	return info, nodeInfos, nil
}

func (st *State) startUpgrade(
	ctx context.Context,
	tx *sqlair.TX,
	controllerID string,
	previousVersion version.Number,
	targetVersion version.Number,
) (Info, []InfoControllerNode, error) {
	q := `
INSERT INTO upgrade_info (uuid, previous_version, target_version, init_time) VALUES
($M.uuid, $M.previousVersion, $M.targetVersion, DATETIME('now'))`
	s, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return Info{}, nil, errors.Annotatef(err, "preparing %q", q)
	}
	err = tx.Query(ctx, s, sqlair.M{
		"uuid":            utils.MustNewUUID().String(),
		"previousVersion": previousVersion.String(),
		"targetVersion":   targetVersion.String(),
	}).Run()
	if err != nil {
		return Info{}, nil, errors.Trace(err)
	}

	info, nodeInfos, err := st.getCurrentUpgrade(ctx, tx)
	if err != nil {
		return Info{}, nil, errors.Trace(err)
	}
	info, nodeInfos, err = st.ensureNodeReady(ctx, tx, info, nodeInfos, controllerID)
	if err != nil {
		return Info{}, nil, errors.Trace(err)
	}
	return info, nodeInfos, nil

}

func (st *State) ensureNodeReady(
	ctx context.Context,
	tx *sqlair.TX,
	info Info,
	nodeInfos []InfoControllerNode,
	controllerID string,
) (Info, []InfoControllerNode, error) {
	for _, nodeInfo := range nodeInfos {
		if nodeInfo.ControllerNodeID == controllerID {
			return info, nodeInfos, nil
		}
	}
	q := `
INSERT INTO upgrade_info_controller_node (uuid, controller_node_id, upgrade_info_uuid, upgrade_node_status_id) VALUES
($M.uuid, $M.controllerID, $M.infoUUID, $M.readyKey)`
	s, err := sqlair.Prepare(q, sqlair.M{})
	if err != nil {
		return Info{}, nil, errors.Annotatef(err, "preparing %q", q)
	}
	err = tx.Query(ctx, s, sqlair.M{
		"uuid":         utils.MustNewUUID().String(),
		"controllerID": controllerID,
		"infoUUID":     info.UUID,
		"readyKey":     0,
	}).Run()
	if err != nil {
		return Info{}, nil, errors.Trace(err)
	}
	nodeInfos = append(nodeInfos, InfoControllerNode{ControllerNodeID: controllerID, NodeStatus: "ready"})
	return info, nodeInfos, nil
}

func verifyVersionMatches(info Info, previousVersion, targetVersion version.Number) error {
	if info.PreviousVersion != previousVersion.String() || info.TargetVersion != targetVersion.String() {
		return errors.NotValidf(
			"current upgrade (%s -> %s) does not match started upgrade (%s -> %s)",
			info.PreviousVersion, info.TargetVersion, previousVersion, targetVersion,
		)
	}
	return nil
}

// IsUpgrading returns true if an upgrade is currently in progress.
func (st *State) IsUpgrading(ctx context.Context) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}
	var upgrading bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, _, err := st.getCurrentUpgrade(ctx, tx)
		if err == nil {
			upgrading = true
			return nil
		}
		if err == noUpgradeErr {
			upgrading = false
			return nil
		}
		return errors.Trace(err)
	})
	return upgrading, errors.Trace(err)
}
