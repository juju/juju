// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	coredatabase "github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
)

// ControllerState is a reference to the underlying data accessor for data
// in the controller database.
type ControllerState struct {
	*domain.StateBase
}

// NewControllerState creates a new state struct for querying the controller state.
func NewControllerState(factory coredatabase.TxnRunnerFactory) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetModelSecretBackend sets the secret backend for the given model.
func (s *ControllerState) SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	q := `
SELECT b.uuid AS &SecretBackendInfo.uuid
FROM   secret_backend b
WHERE  b.name =
    CASE $SecretBackendInfo.name
    WHEN 'auto' THEN
        CASE (
            SELECT mt.type FROM model_type mt
            JOIN   model m on mt.id = m.model_type_id
            WHERE  m.uuid = $SecretBackendInfo.model_uuid
        )
        WHEN 'iaas' THEN 'internal'
        WHEN 'caas' THEN 'kubernetes'
        END
    ELSE
        $SecretBackendInfo.name
    END
`
	backendInfo := SecretBackendInfo{Name: backendName, ModelUUID: modelUUID.String()}
	stmt, err := s.Prepare(q, backendInfo)
	if err != nil {
		return errors.Trace(err)
	}

	modelBackendUpsert := `
INSERT INTO model_secret_backend(model_uuid, secret_backend_uuid)
VALUES ($SecretBackendInfo.model_uuid, $SecretBackendInfo.uuid)
ON CONFLICT(model_uuid) DO UPDATE SET
    secret_backend_uuid = EXCLUDED.secret_backend_uuid`
	modelBackendUpsertStmt, err := s.Prepare(modelBackendUpsert, backendInfo)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, backendInfo).Get(&backendInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", backenderrors.NotFound, backendName)
		}
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		err = tx.Query(ctx, modelBackendUpsertStmt, backendInfo).Run()
		if database.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: model %q", modelerrors.NotFound, modelUUID)
		}
		if err != nil {
			return fmt.Errorf("setting secret backend %q for model %q: %w", backendName, modelUUID, err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// GetModelSecretBackendName returns the secret backend name
// for a given model uuid.
func (s *ControllerState) GetModelSecretBackendName(ctx context.Context, modelUUID coremodel.UUID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	backendInfo := SecretBackendInfo{ModelUUID: modelUUID.String()}

	stmt, err := s.Prepare(`
SELECT sb.name AS &SecretBackendInfo.name
FROM   model_secret_backend msb
JOIN   secret_backend sb ON sb.uuid = msb.secret_backend_uuid
WHERE  model_uuid = $SecretBackendInfo.model_uuid`, backendInfo)
	if err != nil {
		return "", errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, backendInfo).Get(&backendInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("secret backend for model %q not found%w", modelUUID, errors.Hide(backenderrors.NotFound))
		}
		return errors.Trace(err)
	})
	if err != nil {
		return "", errors.Trace(domain.CoerceError(err))
	}
	return backendInfo.Name, nil
}
