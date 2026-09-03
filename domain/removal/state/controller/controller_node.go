// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/internal/errors"
)

// DeleteDqliteNode removes the controller node identified by controllerID
// and all records that depend on it.
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
	deleteNodeStmt, err := st.Prepare(`
DELETE FROM controller_node
WHERE controller_id = $controllerNode.controller_id`, node)
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
		if err := tx.Query(ctx, deleteNodeStmt, node).Run(); err != nil {
			return errors.Errorf("deleting controller node %q: %w", controllerID, err)
		}
		return nil
	}))
}
