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

	"github.com/juju/juju/cloud"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	cloudstate "github.com/juju/juju/domain/cloud/state"
	"github.com/juju/juju/domain/credential"
	credentialstate "github.com/juju/juju/domain/credential/state"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/secretbackend"
	domainsecretbackend "github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// State represents database interactions dealing with secret backends.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new secret backend state based on the input database factory method.
func NewState(factory coredatabase.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// GetModelSecretBackendDetails is responsible for returning the backend
// details for a given model uuid.
func (s *State) GetModelSecretBackendDetails(ctx context.Context, uuid coremodel.UUID) (secretbackend.ModelSecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}

	stmt, err := s.Prepare(`
SELECT &ModelSecretBackend.*
FROM   v_model_secret_backend
WHERE  uuid = $M.uuid`, sqlair.M{}, ModelSecretBackend{})
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}
	var m ModelSecretBackend
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, sqlair.M{"uuid": uuid}).Get(&m)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q", modelerrors.NotFound, uuid)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(domain.CoerceError(err))
	}
	if !m.Type.IsValid() {
		// This should never happen.
		return secretbackend.ModelSecretBackend{}, fmt.Errorf("invalid model type for model %q", m.Name)
	}
	return secretbackend.ModelSecretBackend{
		ControllerUUID:  m.ControllerUUID,
		ID:              m.ID,
		Name:            m.Name,
		Type:            m.Type,
		SecretBackendID: m.SecretBackendID,
	}, nil
}

// CreateSecretBackend creates a new secret backend.
func (s *State) CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error) {
	if err := params.Validate(); err != nil {
		return "", errors.Trace(err)
	}

	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		_, err := s.upsertSecretBackend(ctx, tx, upsertSecretBackendParams{
			ID:                  params.ID,
			Name:                params.Name,
			BackendType:         params.BackendType,
			TokenRotateInterval: params.TokenRotateInterval,
			NextRotateTime:      params.NextRotateTime,
			Config:              params.Config,
		})
		return errors.Trace(err)
	})
	if err != nil {
		if database.IsErrConstraintUnique(err) {
			return "", fmt.Errorf("%w: secret backend with name %q", secretbackenderrors.AlreadyExists, params.Name)
		}
		return "", domain.CoerceError(err)
	}
	return params.ID, nil
}

// UpdateSecretBackend updates the secret backend.
func (s *State) UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) (string, error) {
	if err := params.Validate(); err != nil {
		return "", errors.Trace(err)
	}

	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		existing, err := s.getSecretBackend(ctx, tx, params.BackendIdentifier)
		if err != nil {
			return errors.Trace(err)
		}
		upsertParams := upsertSecretBackendParams{
			// secret_backend table.
			ID:                  existing.ID,
			Name:                existing.Name,
			BackendType:         existing.BackendType,
			TokenRotateInterval: existing.TokenRotateInterval,

			// secret_backend_rotation table.
			NextRotateTime: params.NextRotateTime,

			// secret_backend_config table.
			Config: params.Config,
		}
		if params.NewName != nil {
			upsertParams.Name = *params.NewName
		}
		if params.TokenRotateInterval != nil {
			upsertParams.TokenRotateInterval = params.TokenRotateInterval
		}
		_, err = s.upsertSecretBackend(ctx, tx, upsertParams)
		return errors.Trace(err)
	})
	return params.ID, domain.CoerceError(err)
}

func (s *State) upsertSecretBackend(ctx context.Context, tx *sqlair.TX, params upsertSecretBackendParams) (string, error) {
	if err := params.Validate(); err != nil {
		return "", errors.Trace(err)
	}

	backendTypeID, err := domainsecretbackend.MarshallBackendType(params.BackendType)
	if err != nil {
		return "", errors.Trace(err)
	}
	sb := SecretBackend{
		ID:            params.ID,
		Name:          params.Name,
		BackendTypeID: backendTypeID,
	}
	if params.TokenRotateInterval != nil {
		sb.TokenRotateInterval = database.NewNullDuration(*params.TokenRotateInterval)
	}

	if err := s.upsertBackend(ctx, tx, sb); err != nil {
		return params.ID, errors.Trace(err)
	}
	if params.NextRotateTime != nil && !params.NextRotateTime.IsZero() {
		if err := s.upsertBackendRotation(ctx, tx, SecretBackendRotation{
			ID:               params.ID,
			NextRotationTime: sql.NullTime{Time: *params.NextRotateTime, Valid: true},
		}); err != nil {
			return params.ID, errors.Trace(err)
		}
	}
	if len(params.Config) == 0 {
		return params.ID, nil
	}
	if err := s.upsertBackendConfig(ctx, tx, params.ID, params.Config); err != nil {
		return params.ID, errors.Trace(err)
	}
	return params.ID, nil
}

// DeleteSecretBackend deletes the secret backend for the given backend ID.
func (s *State) DeleteSecretBackend(ctx context.Context, identifier secretbackend.BackendIdentifier, deleteInUse bool) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	// TODO: check if the backend is in use. JUJU-5707
	// if !deleteInUse {
	// }
	cfgStmt, err := s.Prepare(`
DELETE FROM secret_backend_config WHERE backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	rotationStmt, err := s.Prepare(`
DELETE FROM secret_backend_rotation WHERE backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(secrets) - use a struct not string literals
	modelSecretBackendStmt, err := s.Prepare(`
UPDATE model_secret_backend
SET secret_backend_uuid = (
    SELECT sb.uuid FROM secret_backend sb
    JOIN model m ON m.uuid = model_secret_backend.model_uuid
    JOIN model_type mt ON mt.id = m.model_type_id
    WHERE sb.name =
    CASE mt.type
    WHEN 'iaas' THEN 'internal'
    WHEN 'caas' THEN 'kubernetes'
    END
)
WHERE secret_backend_uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	backendStmt, err := s.Prepare(`
DELETE FROM secret_backend WHERE uuid = $M.uuid`, sqlair.M{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if identifier.ID == "" {
			sb, err := s.getSecretBackend(ctx, tx, identifier)
			if err != nil {
				return errors.Trace(err)
			}
			identifier.ID = sb.ID
		}

		arg := sqlair.M{"uuid": identifier.ID}
		if err := tx.Query(ctx, cfgStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend config for %q: %w", identifier.ID, err)
		}
		if err := tx.Query(ctx, rotationStmt, arg).Run(); err != nil {
			return fmt.Errorf("deleting secret backend rotation for %q: %w", identifier.ID, err)
		}
		if err = tx.Query(ctx, modelSecretBackendStmt, arg).Run(); err != nil {
			return fmt.Errorf("resetting secret backend %q to NULL for models: %w", identifier.ID, err)
		}
		err = tx.Query(ctx, backendStmt, arg).Run()
		if database.IsErrConstraintTrigger(err) {
			return fmt.Errorf("%w: %q is immutable", secretbackenderrors.Forbidden, identifier.ID)
		}
		if err != nil {
			return fmt.Errorf("deleting secret backend for %q: %w", identifier.ID, err)
		}
		return nil
	})
	return domain.CoerceError(err)
}

// ListSecretBackends returns a list of all secret backends which contain secrets.
func (s *State) ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO: implement for inUse. JUJU-5707
	stmt, err := s.Prepare(`
SELECT
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    bt.type                 AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    JOIN secret_backend_type bt ON b.backend_type_id = bt.id
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
ORDER BY b.name`, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			// We do not want to return an error if there are no secret backends.
			// We just return an empty list.
			s.logger.Debugf("no secret backends found")
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
	backends := rows.toSecretBackends()

	var result []*secretbackend.SecretBackend
	for _, b := range backends {
		if b.Name == kubernetes.BackendName {
			// TODO(secrets) - count the secrets
			//if numSecrets == 0 {
			//	continue
			//}
		}
		result = append(result, b)
	}
	return result, errors.Trace(err)
}

// ListSecretBackendsForModel returns a list of all secret backends
// which contain secrets for the specified model, unless includeEmpty is true
// in which case all backends are returned.
func (s *State) ListSecretBackendsForModel(ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// TODO: implement for inUse. JUJU-5707
	stmt, err := s.Prepare(`
SELECT
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    bt.type                 AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    JOIN secret_backend_type bt ON b.backend_type_id = bt.id
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
ORDER BY b.name`, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	getModelStmt, err := s.Prepare(`
SELECT mt.type AS &modelDetails.model_type
FROM   model m
JOIN   model_type mt ON mt.id = model_type_id
WHERE  m.uuid = $M.uuid
`, modelDetails{}, sqlair.M{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var (
		rows      SecretBackendRows
		modelType coremodel.ModelType
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var modelDetails modelDetails
		err := tx.Query(ctx, getModelStmt, sqlair.M{"uuid": modelUUID}).Get(&modelDetails)
		if errors.Is(err, sql.ErrNoRows) {
			// Should never happen.
			return fmt.Errorf("%w: %s", modelerrors.NotFound, modelUUID)
		}
		if err != nil {
			return fmt.Errorf("querying model: %w", err)
		}
		modelType = modelDetails.Type

		err = tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			// We do not want to return an error if there are no secret backends.
			// We just return an empty list.
			s.logger.Debugf("no secret backends found")
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
	backends := rows.toSecretBackends()

	var result []*secretbackend.SecretBackend
	for _, b := range backends {
		if modelType == coremodel.CAAS && b.Name == juju.BackendName {
			continue
		}
		// TODO(secrets) - count the secrets
		//if numSecrets == 0 && !includeEmpty {
		//	continue
		//}
		result = append(result, b)
	}
	return result, errors.Trace(err)
}

func (s *State) getSecretBackend(ctx context.Context, tx *sqlair.TX, identifier secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error) {
	if identifier.ID == "" && identifier.Name == "" {
		return nil, fmt.Errorf("%w: both ID and name are missing", secretbackenderrors.NotValid)
	}
	if identifier.ID != "" && identifier.Name != "" {
		return nil, fmt.Errorf("%w: both ID and name are provided", secretbackenderrors.NotValid)
	}
	columName := "uuid"
	v := identifier.ID
	if identifier.Name != "" {
		columName = "name"
		v = identifier.Name
	}

	q := fmt.Sprintf(`
SELECT
    b.uuid                  AS &SecretBackendRow.uuid,
    b.name                  AS &SecretBackendRow.name,
    bt.type                 AS &SecretBackendRow.backend_type,
    b.token_rotate_interval AS &SecretBackendRow.token_rotate_interval,
    c.name                  AS &SecretBackendRow.config_name,
    c.content               AS &SecretBackendRow.config_content
FROM secret_backend b
    JOIN secret_backend_type bt ON b.backend_type_id = bt.id
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
WHERE b.%s = $M.identifier`, columName)
	stmt, err := s.Prepare(q, sqlair.M{}, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows SecretBackendRows
	err = tx.Query(ctx, stmt, sqlair.M{"identifier": v}).GetAll(&rows)
	if errors.Is(err, sql.ErrNoRows) || len(rows) == 0 {
		return nil, fmt.Errorf("%w: %q", secretbackenderrors.NotFound, v)
	}
	if err != nil {
		return nil, fmt.Errorf("querying secret backends: %w", err)
	}
	return rows.toSecretBackends()[0], nil
}

// GetSecretBackend returns the secret backend for the given backend ID or Name.
func (s *State) GetSecretBackend(ctx context.Context, params secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error) {
	if params.ID == "" && params.Name == "" {
		return nil, fmt.Errorf("%w: both ID and name are missing", secretbackenderrors.NotValid)
	}
	if params.ID != "" && params.Name != "" {
		return nil, fmt.Errorf("%w: both ID and name are provided", secretbackenderrors.NotValid)
	}
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var sb *secretbackend.SecretBackend
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		sb, err = s.getSecretBackend(ctx, tx, params)
		return errors.Trace(err)
	})
	return sb, domain.CoerceError(err)
}

// SecretBackendRotated updates the next rotation time for the secret backend.
func (s *State) SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}
	updateStmt, err := s.Prepare(`
UPDATE secret_backend_rotation
SET next_rotation_time = $SecretBackendRotation.next_rotation_time
WHERE backend_uuid = $SecretBackendRotation.backend_uuid
    AND $SecretBackendRotation.next_rotation_time < next_rotation_time`, SecretBackendRotation{})
	if err != nil {
		return errors.Trace(err)
	}
	getStmt, err := s.Prepare(`
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
			return fmt.Errorf("%w: %q", secretbackenderrors.NotFound, backendID)
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

// GetControllerModelCloudAndCredential returns the cloud and cloud credential for the
// controller model.
func (s *State) GetControllerModelCloudAndCredential(
	ctx context.Context,
) (cloud.Cloud, cloud.Credential, error) {
	db, err := s.DB()
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, errors.Trace(err)
	}

	modelQuery := `
SELECT uuid AS &M.uuid FROM MODEL
WHERE  name = 'controller'
`
	modelStmt, err := s.Prepare(modelQuery, sqlair.M{})
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, errors.Trace(err)
	}

	var (
		cld  cloud.Cloud
		cred cloud.Credential
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err := tx.Query(ctx, modelStmt).Get(&result)
		if errors.Is(err, sql.ErrNoRows) {
			// Should never happen.
			return fmt.Errorf("controller model not found%w", errors.Hide(modelerrors.NotFound))
		}
		if err != nil {
			return fmt.Errorf("querying controller model: %w", err)
		}
		modelID, _ := result["uuid"].(string)
		cld, cred, err = s.getModelCloudAndCredential(ctx, tx, coremodel.UUID(modelID))
		return errors.Trace(err)
	})
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, domain.CoerceError(err)
	}
	return cld, cred, nil
}

// GetModelCloudAndCredential returns the cloud and cloud credential for the
// given model id. If no model is found for the provided id an error of
// [modelerrors.NotFound] is returned.
func (s *State) GetModelCloudAndCredential(
	ctx context.Context,
	modelID coremodel.UUID,
) (cloud.Cloud, cloud.Credential, error) {
	db, err := s.DB()
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, errors.Trace(err)
	}
	var (
		cld  cloud.Cloud
		cred cloud.Credential
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		cld, cred, err = s.getModelCloudAndCredential(ctx, tx, modelID)
		return errors.Trace(err)
	})
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, errors.Trace(err)
	}
	return cld, cred, nil
}

func (s *State) getModelCloudAndCredential(
	ctx context.Context,
	tx *sqlair.TX,
	modelID coremodel.UUID,
) (cloud.Cloud, cloud.Credential, error) {

	q := `
    SELECT (cloud_uuid, cloud_credential_uuid) AS (&modelCloudAndCredentialID.*)
	FROM v_model
	WHERE uuid = $M.model_id
	`

	stmt, err := s.Prepare(q, sqlair.M{}, modelCloudAndCredentialID{})
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, errors.Trace(err)
	}

	args := sqlair.M{
		"model_id": modelID,
	}
	ids := modelCloudAndCredentialID{}

	var (
		cld        cloud.Cloud
		credResult credential.CloudCredentialResult
	)
	err = tx.Query(ctx, stmt, args).Get(&ids)
	if errors.Is(err, sql.ErrNoRows) {
		return cloud.Cloud{}, cloud.Credential{}, fmt.Errorf("%w for id %q", modelerrors.NotFound, modelID)
	} else if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, errors.Trace(err)
	}

	cld, err = cloudstate.GetCloudForID(ctx, s, tx, ids.CloudID)
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, fmt.Errorf("getting model %q cloud for id %q: %w", modelID, ids.CloudID, err)
	}

	credResult, err = credentialstate.GetCloudCredential(ctx, s, tx, ids.CredentialID)
	if err != nil {
		return cloud.Cloud{}, cloud.Credential{}, fmt.Errorf("getting model %q cloud credential for id %q: %w", modelID, ids.CredentialID, err)
	}
	return cld, cloud.NewNamedCredential(credResult.Label, cloud.AuthType(credResult.AuthType), credResult.Attributes, credResult.Revoked), nil
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

	stmt, err := s.Prepare(`
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
			// We do not want to trigger anything in this case.
			return nil
		}
		if err != nil {
			return fmt.Errorf("querying secret backend rotation changes: %w", err)
		}
		return nil
	})
	return rows.toChanges(s.logger), errors.Trace(err)
}
