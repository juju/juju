// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
)

// Logger is the interface used by the state to log messages.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// State represents database interactions dealing with secret backends.
type State struct {
	*domain.StateBase
	logger Logger
}

// NewState returns a new secret backend state based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory, logger Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetModel is responsible for returning the model for the provided uuid. If no
// model is found the given uuid then an error of
// [github.com/juju/juju/domain/model/errors.NotFound] is returned.
func (s *State) GetModel(ctx context.Context, uuid coremodel.UUID) (*secretbackend.Model, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT (
	uuid, name, model_type_type, secret_backend_uuid
) AS (&Model.*)
FROM v_model
WHERE uuid = $M.uuid`, sqlair.M{}, Model{})
	if err != nil {
		return nil, errors.Trace(err)
	}
	var m Model
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlair.M{"uuid": uuid}).Get(&m)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", modelerrors.NotFound, uuid)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(domain.CoerceError(err))
	}
	if !m.Type.IsValid() {
		// This should never happen.
		return nil, fmt.Errorf("invalid model type for model %q", m.Name)
	}
	return &secretbackend.Model{
		ID:              m.ID,
		Name:            m.Name,
		Type:            m.Type,
		SecretBackendID: m.SecretBackendID,
	}, nil
}

// UpsertSecretBackend persists the input secret backend entity.
func (s *State) UpsertSecretBackend(ctx context.Context, params secretbackend.UpsertSecretBackendParams) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	operation := upsertOperation{UpsertSecretBackendParams: params}
	if err := operation.build(); err != nil {
		return "", errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return errors.Trace(operation.apply(ctx, tx))
	})
	return params.ID, domain.CoerceError(err)
}

// DeleteSecretBackend deletes the secret backend for the given backend ID.
func (s *State) DeleteSecretBackend(ctx context.Context, backendID string, force bool) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	// TODO: check if the backend is in use. JUJU-5707
	// if !force {
	// }
	cfgStmt, err := sqlair.Prepare(`
DELETE FROM secret_backend_config WHERE backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	rotationStmt, err := sqlair.Prepare(`
DELETE FROM secret_backend_rotation WHERE backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	// TODO: we should set it to the `default` backend once we start to include the
	// `internal` and `k8s` backends in the database.
	// For now, we reset it to NULL. JUJU-5708
	modelMetadataStmt, err := sqlair.Prepare(`
UPDATE model_metadata
SET secret_backend_uuid = NULL
WHERE secret_backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	backendStmt, err := sqlair.Prepare(`
DELETE FROM secret_backend WHERE uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		arg := sqlair.M{"uuid": backendID}
		if err := tx.Query(ctx, cfgStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend config for %q: %w", backendID, err)
		}
		if err := tx.Query(ctx, rotationStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend rotation for %q: %w", backendID, err)
		}
		if err = tx.Query(ctx, modelMetadataStmt, arg).Run(); err != nil {
			return fmt.Errorf("resetting secret backend %q to NULL for model metadata: %w", backendID, err)
		}
		if err = tx.Query(ctx, backendStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend for %q: %w", backendID, err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// ListSecretBackends returns a list of all secret backends.
func (s *State) ListSecretBackends(ctx context.Context) ([]*coresecrets.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stmt, err := sqlair.Prepare(`
SELECT 
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    b.backend_type          AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
ORDER BY b.name`, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			// We donnot want to return an error if there are no secret backends.
			// We just return an empty list.
			return nil
		}
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot list secret backends: %w", err)
	}
	return rows.toSecretBackends(), errors.Trace(err)
}

func (s *State) getSecretBackend(ctx context.Context, columName string, v string) (*coresecrets.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	q := fmt.Sprintf(`
SELECT
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    b.backend_type          AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
WHERE b.%s = $M.identifier`, columName)
	stmt, err := sqlair.Prepare(q, sqlair.M{}, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlair.M{"identifier": v}).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) || len(rows) == 0 {
			return fmt.Errorf("%w: %q", backenderrors.NotFound, v)
		}
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		return nil
	})
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", backenderrors.NotFound, v)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return rows.toSecretBackends()[0], errors.Trace(err)
}

// GetSecretBackendByName returns the secret backend for the given backend name.
func (s *State) GetSecretBackendByName(ctx context.Context, backendName string) (*coresecrets.SecretBackend, error) {
	return s.getSecretBackend(ctx, "name", backendName)
}

// GetSecretBackend returns the secret backend for the given backend ID.
func (s *State) GetSecretBackend(ctx context.Context, backendID string) (*coresecrets.SecretBackend, error) {
	return s.getSecretBackend(ctx, "uuid", backendID)
}

// SecretBackendRotated updates the next rotation time for the secret backend.
func (s *State) SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	updateStmt, err := sqlair.Prepare(`
UPDATE secret_backend_rotation
SET next_rotation_time = $SecretBackendRotation.next_rotation_time
WHERE backend_uuid = $SecretBackendRotation.backend_uuid
    AND $SecretBackendRotation.next_rotation_time < next_rotation_time`, SecretBackendRotation{})
	if err != nil {
		return errors.Trace(err)
	}
	getStmt, err := sqlair.Prepare(`
SELECT uuid AS &SecretBackendRotationRow.uuid
FROM secret_backend
WHERE uuid = $M.uuid`, sqlair.M{}, SecretBackendRotationRow{})
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the secret backend exists.
		var sb SecretBackendRotationRow
		err := tx.Query(ctx, getStmt, sqlair.M{"uuid": backendID}).Get(&sb)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", backenderrors.NotFound, backendID)
		}
		if err != nil {
			return fmt.Errorf("checking if secret backend %q exists: %w", backendID, err)
		}

		err = tx.Query(ctx, updateStmt,
			SecretBackendRotation{
				ID:               backendID,
				NextRotationTime: sql.NullTime{Time: next, Valid: true},
			},
		).Run()
		if err != nil {
			return fmt.Errorf("updating secret backend rotation: %w", err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// InitialWatchStatement returns the initial watch statement and the table name to watch.
func (s *State) InitialWatchStatement() (string, string) {
	return "secret_backend_rotation", "SELECT backend_uuid FROM secret_backend_rotation"
}

// GetSecretBackendRotateChanges returns the secret backend rotation changes
// for the given backend IDs for the Watcher.
func (s *State) GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	stmt, err := sqlair.Prepare(`
SELECT 
    b.uuid               AS &SecretBackendRotationRow.uuid,
    b.name               AS &SecretBackendRotationRow.name,
    r.next_rotation_time AS &SecretBackendRotationRow.next_rotation_time
FROM secret_backend b
    LEFT JOIN secret_backend_rotation r ON b.uuid = r.backend_uuid
WHERE b.uuid IN ($S[:])`,
		sqlair.S{}, SecretBackendRotationRow{},
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRotationRows
	args := sqlair.S(transform.Slice(backendIDs, func(s string) any { return any(s) }))
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, args).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			// This can happen only if the backends were deleted immediately after the rotation gets updated.
			// We donnot want to trigger anything in this case.
			return nil
		}
		if err != nil {
			return fmt.Errorf("querying secret backend rotation changes: %w", err)
		}
		return nil
	})
	return rows.toChanges(s.logger), errors.Trace(err)
}
