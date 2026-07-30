// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/internal/errors"
)

// ControllerState defines the access mechanism for interacting with passwords
// in the context of the controller database.
type ControllerState struct {
	*domain.StateBase
}

// NewControllerState constructs a new state for interacting with the underlying
// passwords of a controller.
func NewControllerState(factory database.TxnRunnerFactory) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetControllerNodePasswordHash sets the password hash for the given unit.
func (s *ControllerState) SetControllerNodePasswordHash(ctx context.Context, id string, passwordHash agentpassword.PasswordHash) error {
	db, err := s.DB(ctx)
	if err != nil {
		return err
	}

	args := entityPasswordHash{
		UUID:         id,
		PasswordHash: passwordHash,
	}

	ensureControllerNodeQuery := `
INSERT INTO controller_node (controller_id)
VALUES ($entityPasswordHash.uuid)
ON CONFLICT (controller_id) DO NOTHING;
`
	ensureControllerNodeStmt, err := s.Prepare(ensureControllerNodeQuery, args)
	if err != nil {
		return errors.Errorf("preparing statement to ensure controller node exists: %w", err)
	}

	insertQuery := `
INSERT INTO controller_node_password (controller_id, password_hash_algorithm_id, password_hash)
VALUES ($entityPasswordHash.uuid, 0, $entityPasswordHash.password_hash)
ON CONFLICT (controller_id) DO UPDATE
SET password_hash = $entityPasswordHash.password_hash;
`
	insertStmt, err := s.Prepare(insertQuery, args)
	if err != nil {
		return errors.Errorf("preparing statement to set password hash: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, ensureControllerNodeStmt, args).Run(); err != nil {
			return errors.Errorf("ensuring controller node exists: %w", err)
		}

		if err := tx.Query(ctx, insertStmt, args).Run(); err != nil {
			return errors.Errorf("setting password hash: %w", err)
		}
		return nil
	})
	return errors.Capture(err)
}

// MatchesControllerNodePasswordHash checks if the password is valid or not against the
// password hash stored in the database.
func (s *ControllerState) MatchesControllerNodePasswordHash(ctx context.Context, id string, passwordHash agentpassword.PasswordHash) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, err
	}

	args := validatePasswordHash{
		UUID:         id,
		PasswordHash: passwordHash,
	}

	query := `
SELECT COUNT(*) AS &validatePasswordHash.count
FROM   controller_node_password
WHERE  controller_id = $validatePasswordHash.uuid
AND    password_hash = $validatePasswordHash.password_hash;
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return false, errors.Errorf("preparing statement to set password hash: %w", err)
	}

	var count int
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, stmt, args).Get(&args); err != nil {
			return errors.Errorf("setting password hash: %w", err)
		}
		count = args.Count
		return nil
	})
	return count > 0, errors.Capture(err)
}
