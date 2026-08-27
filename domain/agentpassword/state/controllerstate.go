// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentpassword"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
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

	query := `
SELECT COUNT(*) AS &count.count
FROM controller_node
WHERE controller_id = $entityPasswordHash.uuid;
`
	stmt, err := s.Prepare(query, args, count{})
	if err != nil {
		return errors.Errorf("preparing statement to check if password hash exists: %w", err)
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
		var count count
		if err := tx.Query(ctx, stmt, args).Get(&count); count.Count == 0 {
			return errors.Errorf("controller node %q: %w", id, controllernodeerrors.NotFound)
		} else if err != nil {
			return errors.Errorf("checking if password hash exists: %w", err)
		}

		if err := tx.Query(ctx, insertStmt, args).Run(); err != nil {
			return errors.Errorf("setting password hash: %w", err)
		}
		return nil
	})
	return errors.Capture(err)
}

// SetControllerNodePasswordHashIfAbsent sets the password hash for the given
// controller node only if it does not already have one.
func (s *ControllerState) SetControllerNodePasswordHashIfAbsent(
	ctx context.Context, id string, passwordHash agentpassword.PasswordHash,
) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	args := entityPasswordHash{
		UUID:         id,
		PasswordHash: passwordHash,
	}
	checkNodeStmt, err := s.Prepare(`
SELECT COUNT(*) AS &count.count
FROM controller_node
WHERE controller_id = $entityPasswordHash.uuid;
`, args, count{})
	if err != nil {
		return false, errors.Errorf("preparing statement to check controller node exists: %w", err)
	}
	insertStmt, err := s.Prepare(`
INSERT INTO controller_node_password (controller_id, password_hash_algorithm_id, password_hash)
VALUES ($entityPasswordHash.uuid, 0, $entityPasswordHash.password_hash)
ON CONFLICT (controller_id) DO NOTHING;
`, args)
	if err != nil {
		return false, errors.Errorf("preparing statement to initialize password hash: %w", err)
	}

	var inserted bool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		inserted = false
		var count count
		if err := tx.Query(ctx, checkNodeStmt, args).Get(&count); err != nil {
			return errors.Errorf("checking controller node exists: %w", err)
		} else if count.Count == 0 {
			return errors.Errorf("controller node %q: %w", id, controllernodeerrors.NotFound)
		}

		outcome := sqlair.Outcome{}
		if err := tx.Query(ctx, insertStmt, args).Get(&outcome); err != nil {
			return errors.Errorf("initializing password hash: %w", err)
		}
		rowsAffected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Errorf("checking initialized password hash: %w", err)
		}
		inserted = rowsAffected == 1
		return nil
	})
	return inserted, errors.Capture(err)
}

// HasControllerNodePasswordHash reports whether the controller node has a
// password hash.
func (s *ControllerState) HasControllerNodePasswordHash(ctx context.Context, id string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	args := entityPasswordHash{UUID: id}
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &count.count
FROM controller_node_password
WHERE controller_id = $entityPasswordHash.uuid;
`, args, count{})
	if err != nil {
		return false, errors.Errorf("preparing statement to check password hash exists: %w", err)
	}

	var result count
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = count{}
		return errors.Capture(tx.Query(ctx, stmt, args).Get(&result))
	})
	return result.Count > 0, errors.Capture(err)
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

type controllerNonce struct {
	ControllerID string `db:"controller_id"`
	Nonce        string `db:"nonce"`
}

type controllerNonceCount struct {
	Count int `db:"count"`
}

// EnsureControllerNodeNonce returns the nonce for a controller ID, creating it
// from nonce when it does not already exist. Once assigned, a nonce is never
// overwritten: callers may safely retry after a partial reconciliation.
func (s *ControllerState) EnsureControllerNodeNonce(ctx context.Context, controllerID, nonce string) (string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	args := controllerNonce{
		ControllerID: controllerID,
		Nonce:        nonce,
	}
	insertStmt, err := s.Prepare(`
INSERT INTO controller_node_nonce (controller_id, nonce)
VALUES ($controllerNonce.controller_id, $controllerNonce.nonce)
ON CONFLICT (controller_id) DO NOTHING;
`, args)
	if err != nil {
		return "", errors.Errorf("preparing statement to ensure controller node nonce: %w", err)
	}

	result := controllerNonce{ControllerID: controllerID}
	getStmt, err := s.Prepare(`
SELECT cnn.nonce AS &controllerNonce.nonce
FROM controller_node_nonce AS cnn
WHERE cnn.controller_id = $controllerNonce.controller_id;
`, result)
	if err != nil {
		return "", errors.Errorf("preparing statement to get controller node nonce: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, insertStmt, args).Run(); err != nil {
			return errors.Capture(err)
		}
		return errors.Capture(tx.Query(ctx, getStmt, result).Get(&result))
	})
	return result.Nonce, errors.Capture(err)
}

// ValidateControllerNodeNonce checks that the given nonce matches the stored
// nonce for the controller ID and returns true if it does. The nonce is not
// consumed; idempotency is provided by the password insert-if-absent guard.
// Returns false if the nonce does not match or no nonce is stored.
func (s *ControllerState) ValidateControllerNodeNonce(ctx context.Context, controllerID, nonce string) (bool, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	args := controllerNonce{
		ControllerID: controllerID,
		Nonce:        nonce,
	}
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &controllerNonceCount.count
FROM controller_node_nonce
WHERE controller_id = $controllerNonce.controller_id
AND nonce = $controllerNonce.nonce;
`, args, controllerNonceCount{})
	if err != nil {
		return false, errors.Errorf("preparing statement to validate controller node nonce: %w", err)
	}

	var result controllerNonceCount
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result = controllerNonceCount{}
		return errors.Capture(tx.Query(ctx, stmt, args).Get(&result))
	})
	return result.Count > 0, errors.Capture(err)
}
