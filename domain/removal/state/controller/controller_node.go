// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// DeleteDqliteNode marks the controller node identified by controllerID dead
// and removes all records that depend on it. The controller node is retained
// as a tombstone so delayed controller workers cannot recreate it.
func (st *State) DeleteDqliteNode(ctx context.Context, controllerID string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	node := controllerNode{ControllerID: controllerID}

	checkExistsStmt, err := st.Prepare(`
SELECT COUNT(*) AS &count.count
FROM controller_node
WHERE controller_id = $controllerNode.controller_id`, node, count{})
	if err != nil {
		return errors.Errorf("preparing controller node existence check: %w", err)
	}
	markNodeDyingStmt, err := st.Prepare(`
UPDATE controller_node
SET life_id = 1
WHERE controller_id = $controllerNode.controller_id
AND life_id = 0`, node)
	if err != nil {
		return errors.Errorf("preparing controller node dying transition: %w", err)
	}

	deleteAPIAddressesStmt, err := st.Prepare(`
DELETE FROM controller_api_address
WHERE controller_id = $controllerNode.controller_id`, node)
	if err != nil {
		return errors.Errorf("preparing controller api address deletion: %w", err)
	}
	deleteAgentVersionStmt, err := st.Prepare(`
DELETE FROM controller_node_agent_version
WHERE controller_id = $controllerNode.controller_id`, node)
	if err != nil {
		return errors.Errorf("preparing controller agent version deletion: %w", err)
	}
	deletePasswordStmt, err := st.Prepare(`
DELETE FROM controller_node_password
WHERE controller_id = $controllerNode.controller_id`, node)
	if err != nil {
		return errors.Errorf("preparing controller password deletion: %w", err)
	}
	deleteNonceStmt, err := st.Prepare(`
DELETE FROM controller_node_nonce
WHERE controller_id = $controllerNode.controller_id`, node)
	if err != nil {
		return errors.Errorf("preparing controller nonce deletion: %w", err)
	}
	deleteUpgradeInfoStmt, err := st.Prepare(`
DELETE FROM upgrade_info_controller_node
WHERE controller_node_id = $controllerNode.controller_id`, node)
	if err != nil {
		return errors.Errorf("preparing controller upgrade info deletion: %w", err)
	}
	markNodeDeadStmt, err := st.Prepare(`
UPDATE controller_node
SET life_id = 2,
    dqlite_node_id = NULL,
    dqlite_bind_address = NULL
WHERE controller_id = $controllerNode.controller_id
AND life_id < 2`, node)
	if err != nil {
		return errors.Errorf("preparing controller node deletion: %w", err)
	}

	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var result count
		if err := tx.Query(ctx, checkExistsStmt, node).Get(&result); err != nil {
			return errors.Errorf("checking controller node %q exists: %w", controllerID, err)
		}
		if result.Count == 0 {
			return nil
		}
		if err := tx.Query(ctx, markNodeDyingStmt, node).Run(); err != nil {
			return errors.Errorf("marking controller node %q dying: %w", controllerID, err)
		}

		if err := tx.Query(ctx, deleteAPIAddressesStmt, node).Run(); err != nil {
			return errors.Errorf("deleting controller api addresses for %q: %w", controllerID, err)
		}
		if err := tx.Query(ctx, deleteAgentVersionStmt, node).Run(); err != nil {
			return errors.Errorf("deleting controller agent version for %q: %w", controllerID, err)
		}
		if err := tx.Query(ctx, deletePasswordStmt, node).Run(); err != nil {
			return errors.Errorf("deleting controller password for %q: %w", controllerID, err)
		}
		if err := tx.Query(ctx, deleteNonceStmt, node).Run(); err != nil {
			return errors.Errorf("deleting controller nonce for %q: %w", controllerID, err)
		}
		if err := tx.Query(ctx, deleteUpgradeInfoStmt, node).Run(); err != nil {
			return errors.Errorf("deleting controller upgrade info for %q: %w", controllerID, err)
		}
		if err := tx.Query(ctx, markNodeDeadStmt, node).Run(); err != nil {
			return errors.Errorf("marking controller node %q dead: %w", controllerID, err)
		}
		return nil
	}))
}
