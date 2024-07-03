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
	"github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
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

// GetModelSecretBackend returns the secret backend name for the specified model.
func (s *ControllerState) GetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	var backendName string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelBackendDetails, err := s.getModelSecretBackendDetails(ctx, modelUUID, tx)
		if err != nil {
			return errors.Trace(err)
		}
		backendName = modelBackendDetails.SecretBackendName
		switch modelBackendDetails.Type {
		case coremodel.IAAS:
			if backendName == provider.Internal {
				backendName = provider.Auto
			}
		case coremodel.CAAS:
			if backendName == kubernetes.BackendName {
				backendName = provider.Auto
			}
		default:
			// Should never happen.
			return errors.NotValidf("model type %q", modelBackendDetails.Type)
		}
		return nil
	})
	if err != nil {
		return "", domain.CoerceError(err)
	}
	return backendName, nil
}

func (s *ControllerState) getModelSecretBackendDetails(ctx context.Context, uuid coremodel.UUID, tx *sqlair.TX) (secretbackend.ModelSecretBackend, error) {
	stmt, err := s.Prepare(`
SELECT &ModelSecretBackend.*
FROM   v_model_secret_backend
WHERE  uuid = $M.uuid`, sqlair.M{}, ModelSecretBackend{})
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}
	var m ModelSecretBackend
	err = tx.Query(ctx, stmt, sqlair.M{"uuid": uuid}).Get(&m)
	if errors.Is(err, sql.ErrNoRows) {
		return secretbackend.ModelSecretBackend{}, fmt.Errorf("%w: %q", modelerrors.NotFound, uuid)
	}
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(domain.CoerceError(err))
	}
	if !m.Type.IsValid() {
		// This should never happen.
		return secretbackend.ModelSecretBackend{}, fmt.Errorf("invalid model type for model %q", m.Name)
	}
	return secretbackend.ModelSecretBackend{
		ControllerUUID:    m.ControllerUUID,
		ID:                m.ID,
		Name:              m.Name,
		Type:              m.Type,
		SecretBackendID:   m.SecretBackendID,
		SecretBackendName: m.SecretBackendName,
	}, nil
}

// SetModelSecretBackend sets the secret backend for the given model.
// It returns an error satisfying [backenderrors.NotFound]
// if the secret  backend is not found or [modelerrors.NotFound]
// if the model is not found.
func (s *ControllerState) SetModelSecretBackend(
	ctx context.Context, modelUUID coremodel.UUID, backendName string,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	secretBackendSelectQ := `
SELECT b.uuid AS &SecretBackendInfo.uuid
FROM   secret_backend b
WHERE  b.name = $SecretBackendInfo.name
`
	backendInfo := SecretBackendInfo{Name: backendName, ModelUUID: modelUUID.String()}
	secretBackendSelectStmt, err := s.Prepare(secretBackendSelectQ, backendInfo)
	if err != nil {
		return errors.Trace(err)
	}

	modelBackendUpdate := `
INSERT INTO model_secret_backend
    (model_uuid, secret_backend_uuid)
VALUES ($SecretBackendInfo.model_uuid, $SecretBackendInfo.uuid)
ON CONFLICT (model_uuid) DO UPDATE SET
    secret_backend_uuid = excluded.secret_backend_uuid`
	modelBackendUpdateStmt, err := s.Prepare(modelBackendUpdate, backendInfo)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if backendInfo.Name == provider.Auto {
			modelBackendDetails, err := s.getModelSecretBackendDetails(ctx, modelUUID, tx)
			if err != nil {
				return fmt.Errorf("cannot get model secret backend details for model %q: %w", modelUUID, err)
			}
			switch modelBackendDetails.Type {
			case coremodel.IAAS:
				backendInfo.Name = provider.Internal
			case coremodel.CAAS:
				backendInfo.Name = kubernetes.BackendName
			default:
				// Should never happen.
				return errors.NotValidf("model type %q", modelBackendDetails.Type)
			}
			if backendInfo.Name == modelBackendDetails.SecretBackendName {
				// Nothing to update.
				return nil
			}
		}

		err = tx.Query(ctx, secretBackendSelectStmt, backendInfo).Get(&backendInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", backenderrors.NotFound, backendInfo.Name)
		}
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}

		err = tx.Query(ctx, modelBackendUpdateStmt, backendInfo).Run()
		if database.IsErrConstraintForeignKey(err) {
			return fmt.Errorf("%w: model %q", modelerrors.NotFound, modelUUID)
		}
		if err != nil {
			return fmt.Errorf("setting secret backend %q for model %q: %w", backendInfo.Name, modelUUID, err)
		}
		return nil
	})
	return domain.CoerceError(err)
}
