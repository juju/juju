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
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/secrets/provider"
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

// GetModelSecretBackendDetails is responsible for returning the backend details for a given model uuid,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist.
func (s *State) GetModelSecretBackendDetails(ctx context.Context, uuid coremodel.UUID) (secretbackend.ModelSecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}

	var backend secretbackend.ModelSecretBackend
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		backend, err = s.getModelSecretBackendDetails(ctx, tx, uuid)
		return err
	})
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}
	return backend, nil
}

func (s *State) getModelSecretBackendDetails(ctx context.Context, tx *sqlair.TX, uuid coremodel.UUID) (secretbackend.ModelSecretBackend, error) {
	input := modelIdentifier{ModelID: uuid}
	stmt, err := s.Prepare(`
SELECT &ModelSecretBackend.*
FROM   v_model_secret_backend
WHERE  uuid = $modelIdentifier.uuid`, input, ModelSecretBackend{})
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}
	var backend ModelSecretBackend
	err = tx.Query(ctx, stmt, input).Get(&backend)
	if errors.Is(err, sql.ErrNoRows) {
		return secretbackend.ModelSecretBackend{}, fmt.Errorf("cannot get secret backend for model %q: %w", uuid, modelerrors.NotFound)
	}
	if err != nil {
		return secretbackend.ModelSecretBackend{}, errors.Trace(err)
	}
	return secretbackend.ModelSecretBackend{
		ControllerUUID:    backend.ControllerUUID,
		ModelID:           backend.ModelID,
		ModelName:         backend.ModelName,
		ModelType:         backend.ModelType,
		SecretBackendID:   backend.SecretBackendID,
		SecretBackendName: backend.SecretBackendName,
	}, nil
}

// GetModelType returns the model type for the given model UUID.
func (s *State) GetModelType(ctx context.Context, modelUUID coremodel.UUID) (coremodel.ModelType, error) {
	db, err := s.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	mDetails := modelDetails{UUID: modelUUID}
	stmt, err := s.Prepare(`
SELECT mt.type AS &modelDetails.model_type
FROM   model m
JOIN   model_type mt ON mt.id = model_type_id
WHERE  m.uuid = $modelDetails.uuid
`, modelDetails{})
	if err != nil {
		return "", errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, mDetails).Get(&mDetails)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot get model type for model %q: %w", modelUUID, modelerrors.NotFound)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return mDetails.Type, nil
}

// GetInternalAndActiveBackendUUIDs returns the UUIDs for the internal and active secret backends.
func (s *State) GetInternalAndActiveBackendUUIDs(ctx context.Context, modelUUID coremodel.UUID) (string, string, error) {
	db, err := s.DB()
	if err != nil {
		return "", "", errors.Trace(err)
	}

	activeBackend := ModelSecretBackend{ModelID: modelUUID}
	internalBackend := SecretBackend{Name: juju.BackendName}

	stmtActiveBackend, err := s.Prepare(`
SELECT secret_backend_uuid AS &ModelSecretBackend.secret_backend_uuid
FROM   v_model_secret_backend
WHERE  uuid = $ModelSecretBackend.uuid`, activeBackend)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	stmtBackendUUID, err := s.Prepare(`
SELECT uuid AS &SecretBackend.uuid
FROM   secret_backend
WHERE  name = $SecretBackend.name`, internalBackend)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmtActiveBackend, activeBackend).Get(&activeBackend)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot get active secret backend for model %q: %w", modelUUID, modelerrors.NotFound)
		}

		err = tx.Query(ctx, stmtBackendUUID, internalBackend).Get(&internalBackend)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot get secret backend %q: %w", juju.BackendName, secretbackenderrors.NotFound)
		}
		return errors.Trace(err)
	})
	if err != nil {
		return "", "", errors.Trace(err)
	}
	return internalBackend.ID, activeBackend.SecretBackendID, nil
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
		return "", err
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
	return params.ID, err
}

func (s *State) upsertSecretBackend(ctx context.Context, tx *sqlair.TX, params upsertSecretBackendParams) (string, error) {
	if err := params.Validate(); err != nil {
		return "", errors.Trace(err)
	}

	backendTypeID, err := secretbackend.MarshallBackendType(params.BackendType)
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

	input := SecretBackend{ID: identifier.ID, Name: identifier.Name}

	checkInUseStmt, err := s.Prepare(`
SELECT COUNT(*) AS &Count.num
FROM   secret_backend_reference
WHERE  secret_backend_uuid = $SecretBackend.uuid`, input, Count{})
	if err != nil {
		return errors.Trace(err)
	}

	cfgStmt, err := s.Prepare(`
DELETE FROM secret_backend_config WHERE backend_uuid = $SecretBackend.uuid`, input)
	if err != nil {
		return errors.Trace(err)
	}
	rotationStmt, err := s.Prepare(`
DELETE FROM secret_backend_rotation WHERE backend_uuid = $SecretBackend.uuid`, input)
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
WHERE secret_backend_uuid = $SecretBackend.uuid`, input)
	if err != nil {
		return errors.Trace(err)
	}
	backendStmt, err := s.Prepare(`
DELETE FROM secret_backend WHERE uuid = $SecretBackend.uuid`, input)
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if identifier.ID == "" {
			sb, err := s.getSecretBackend(ctx, tx, identifier)
			if err != nil {
				return errors.Trace(err)
			}
			input.ID = sb.ID
		}

		if !deleteInUse {
			var count Count
			err := tx.Query(ctx, checkInUseStmt, input).Get(&count)
			if err != nil {
				return fmt.Errorf("checking if secret backend %q is in use: %w", input.ID, err)
			}
			if count.Num > 0 {
				return fmt.Errorf("%w: %q is in use", secretbackenderrors.Forbidden, input.ID)
			}
		}

		if err := tx.Query(ctx, cfgStmt, input).Run(); err != nil {
			return fmt.Errorf("deleting secret backend config for %q: %w", input.ID, err)
		}
		if err := tx.Query(ctx, rotationStmt, input).Run(); err != nil {
			return fmt.Errorf("deleting secret backend rotation for %q: %w", input.ID, err)
		}
		if err = tx.Query(ctx, modelSecretBackendStmt, input).Run(); err != nil {
			return fmt.Errorf("resetting secret backend %q to NULL for models: %w", input.ID, err)
		}
		if err := s.removeSecretBackendReferenceForBackend(ctx, tx, input.ID); err != nil {
			return fmt.Errorf("removing secret backend reference for %q: %w", input.ID, err)
		}
		err = tx.Query(ctx, backendStmt, input).Run()
		if database.IsErrConstraintTrigger(err) {
			return fmt.Errorf("%w: %q is immutable", secretbackenderrors.Forbidden, input.ID)
		}
		if err != nil {
			return fmt.Errorf("deleting secret backend for %q: %w", input.ID, err)
		}
		return nil
	})
	return err
}

// ListSecretBackendIDs returns a list of all secret backend ids.
func (s *State) ListSecretBackendIDs(ctx context.Context) ([]string, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	stmt, err := s.Prepare(`
SELECT
    b.uuid AS &SecretBackendRow.uuid
FROM secret_backend b`, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var rows secretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("cannot list secret backend ids: %w", err)
	}

	result := make([]string, len(rows))
	for i, r := range rows {
		result[i] = r.ID
	}
	return result, nil
}

// ListSecretBackends returns a list of all secret backends which contain secrets.
func (s *State) ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}
	nonK8sQuery := fmt.Sprintf(`
SELECT
    b.uuid                                   AS &SecretBackendRow.uuid,
    b.name                                   AS &SecretBackendRow.name,
    bt.type                                  AS &SecretBackendRow.backend_type,
    b.token_rotate_interval                  AS &SecretBackendRow.token_rotate_interval,
    COUNT(DISTINCT sbr.secret_revision_uuid) AS &SecretBackendRow.num_secrets,
    c.name                                   AS &SecretBackendRow.config_name,
    c.content                                AS &SecretBackendRow.config_content
FROM secret_backend b
    JOIN secret_backend_type bt ON b.backend_type_id = bt.id
    LEFT JOIN secret_backend_config c ON b.uuid = c.backend_uuid
    LEFT JOIN secret_backend_reference sbr ON b.uuid = sbr.secret_backend_uuid
WHERE b.name <> '%s'
GROUP BY b.name, c.name`, kubernetes.BackendName)
	nonK8sStmt, err := s.Prepare(nonK8sQuery, SecretBackendRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var result []*secretbackend.SecretBackend
	var nonK8sRows secretBackendRows
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result, err = s.listInUseKubernetesSecretBackends(ctx, tx)
		if err != nil {
			return errors.Trace(err)
		}
		err = tx.Query(ctx, nonK8sStmt).GetAll(&nonK8sRows)
		if errors.Is(err, sql.ErrNoRows) {
			// We do not want to return an error if there are no secret backends.
			// We just return an empty list.
			s.logger.Debugf(ctx, "no secret backends found")
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
	return append(result, nonK8sRows.toSecretBackends()...), nil
}

// listInUseKubernetesSecretBackends returns a list of all kubernetes secret backends which contain secrets.
func (s *State) listInUseKubernetesSecretBackends(ctx context.Context, tx *sqlair.TX) ([]*secretbackend.SecretBackend, error) {
	backendQuery := fmt.Sprintf(`
SELECT
    sbr.secret_backend_uuid                  AS &secretBackendForK8sModelRow.uuid,
    b.name                                   AS &secretBackendForK8sModelRow.name,
    vm.name                                  AS &secretBackendForK8sModelRow.model_name,
    bt.type                                  AS &secretBackendForK8sModelRow.backend_type,
    vc.uuid                                  AS &secretBackendForK8sModelRow.cloud_uuid,
    vcca.uuid                                AS &secretBackendForK8sModelRow.cloud_credential_uuid,
    COUNT(DISTINCT sbr.secret_revision_uuid) AS &secretBackendForK8sModelRow.num_secrets,
    (vc.uuid,
    vc.name,
    vc.endpoint,
    vc.skip_tls_verify,
    vc.is_controller_cloud,
    ccc.ca_cert) AS (&cloudRow.*),
    (vcca.uuid,
    vcca.name,
    vcca.auth_type,
    vcca.revoked,
    vcca.attribute_key,
    vcca.attribute_value) AS (&cloudCredentialRow.*)
FROM secret_backend_reference sbr
    JOIN secret_backend b ON sbr.secret_backend_uuid = b.uuid
    JOIN secret_backend_type bt ON b.backend_type_id = bt.id
    JOIN v_model vm ON sbr.model_uuid = vm.uuid
    JOIN v_cloud_auth vc ON vm.cloud_uuid = vc.uuid
    JOIN cloud_ca_cert ccc ON vc.uuid = ccc.cloud_uuid
    JOIN v_cloud_credential_attributes vcca ON vm.cloud_credential_uuid = vcca.uuid
WHERE b.name = '%s'
GROUP BY vm.name, vcca.attribute_key`, kubernetes.BackendName)
	backendStmt, err := s.Prepare(backendQuery, secretBackendForK8sModelRow{}, cloudRow{}, cloudCredentialRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Controller name is still stored in controller config.
	var controller controllerName
	controllerNameStmt, err := s.Prepare(`
SELECT value AS &controllerName.name FROM v_controller_config WHERE key = 'controller-name'
`, controller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var sbData secretBackendForK8sModelRows
	var cloudData cloudRows
	var credentialData cloudCredentialRows

	err = tx.Query(ctx, backendStmt).GetAll(&sbData, &cloudData, &credentialData)
	if errors.Is(err, sql.ErrNoRows) {
		// We do not want to return an error if there are no secret backends.
		// We just return an empty list.
		s.logger.Debugf(ctx, "no in-use kubernetes secret backends found")
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying kubernetes secret backends: %w", err)
	}

	err = tx.Query(ctx, controllerNameStmt).Get(&controller)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("controller name key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("cannot select controller name")
	}
	return sbData.toSecretBackend(controller.Name, cloudData, credentialData)
}

func getK8sBackendConfig(controllerName, modelName string, cloud cloud.Cloud, cred cloud.Credential) (*provider.BackendConfig, error) {
	spec, err := cloudspec.MakeCloudSpec(cloud, "", &cred)
	if err != nil {
		return nil, errors.Trace(err)
	}
	k8sConfig, err := kubernetes.BuiltInConfig(controllerName, modelName, spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return k8sConfig, nil
}

// ListSecretBackendsForModel returns a list of all secret backends
// which contain secrets for the specified model, unless includeEmpty is true
// in which case all backends are returned.
func (s *State) ListSecretBackendsForModel(ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	inUseCondition := ""
	var nonK8sArgs []any
	if !includeEmpty {
		nonK8sArgs = append(nonK8sArgs, SecretBackendReference{ModelID: modelUUID})
		inUseCondition = fmt.Sprintf(`
    LEFT JOIN secret_backend_reference sbr ON b.uuid = sbr.secret_backend_uuid
WHERE (sbr.model_uuid = $SecretBackendReference.model_uuid OR b.name = '%s') AND b.name <> '%s'`[1:], juju.BackendName, kubernetes.BackendName)
	} else {
		inUseCondition = fmt.Sprintf(`
WHERE b.name <> '%s'`[1:], kubernetes.BackendName)
	}

	nonK8sQuery := fmt.Sprintf(`
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
%s
ORDER BY b.name`, inUseCondition)
	nonK8sStmt, err := s.Prepare(nonK8sQuery, append(nonK8sArgs, SecretBackendRow{})...)
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
		rows              secretBackendRows
		modelType         coremodel.ModelType
		currentK8sBackend *secretbackend.SecretBackend
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

		err = tx.Query(ctx, nonK8sStmt, nonK8sArgs...).GetAll(&rows)
		if errors.Is(err, sql.ErrNoRows) {
			// We do not want to return an error if there are no secret backends.
			// We just return an empty list.
			s.logger.Debugf(ctx, "no secret backends found")
			return nil
		}
		if err != nil {
			return fmt.Errorf("querying secret backends: %w", err)
		}
		if modelType != coremodel.CAAS {
			return nil
		}
		currentK8sBackend, err = s.getK8sSecretBackendForModel(ctx, tx, modelUUID)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, fmt.Errorf("cannot list secret backends: %w", err)
	}

	var result []*secretbackend.SecretBackend
	for _, b := range rows.toSecretBackends() {
		if modelType == coremodel.CAAS && b.Name == juju.BackendName {
			continue
		}
		result = append(result, b)
	}
	if currentK8sBackend != nil {
		result = append(result, currentK8sBackend)
	}
	return result, errors.Trace(err)
}

func (s *State) getK8sSecretBackendForModel(ctx context.Context, tx *sqlair.TX, modelUUID coremodel.UUID) (*secretbackend.SecretBackend, error) {
	stmt, err := s.Prepare(`
SELECT
    vc.uuid       AS &secretBackendForK8sModelRow.cloud_uuid,
    vcca.uuid     AS &secretBackendForK8sModelRow.cloud_credential_uuid,
    vm.name       AS &modelDetails.name,
    vm.model_type AS &modelDetails.model_type,
    (vc.uuid,
    vc.name,
    vc.endpoint,
    vc.skip_tls_verify,
    vc.is_controller_cloud,
    ccc.ca_cert) AS (&cloudRow.*),
    (vcca.uuid,
    vcca.name,
    vcca.auth_type,
    vcca.revoked,
    vcca.attribute_key,
    vcca.attribute_value) AS (&cloudCredentialRow.*)
FROM v_model vm
    JOIN v_cloud_auth vc ON vm.cloud_uuid = vc.uuid
    JOIN cloud_ca_cert ccc ON vc.uuid = ccc.cloud_uuid
    JOIN v_cloud_credential_attributes vcca ON vm.cloud_credential_uuid = vcca.uuid
WHERE vm.uuid = $modelIdentifier.uuid
GROUP BY vm.name, vcca.attribute_key`, modelIdentifier{}, modelDetails{}, secretBackendForK8sModelRow{}, cloudRow{}, cloudCredentialRow{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Controller name is still stored in controller config.
	var controller controllerName
	controllerNameStmt, err := s.Prepare(`
SELECT value AS &controllerName.name FROM v_controller_config WHERE key = 'controller-name'
`, controller)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var models []modelDetails
	var sbCloudCredentialIDs secretBackendForK8sModelRows
	var clds cloudRows
	var creds cloudCredentialRows
	err = tx.Query(ctx, stmt, modelIdentifier{ModelID: modelUUID}).GetAll(&models, &sbCloudCredentialIDs, &clds, &creds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %q", modelerrors.NotFound, modelUUID)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot fetch k8s secret backend credential for model %q: %w", modelUUID, err)
	}
	if len(models) == 0 || len(sbCloudCredentialIDs) == 0 {
		return nil, fmt.Errorf("%w: %q", modelerrors.NotFound, modelUUID)
	}
	err = tx.Query(ctx, controllerNameStmt).Get(&controller)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("controller name key not found")
	}
	if err != nil {
		return nil, fmt.Errorf("cannot select controller name")
	}
	model := models[0]
	if model.Type != coremodel.CAAS {
		return nil, fmt.Errorf("%w: %q", modelerrors.NotFound, modelUUID)
	}
	sbCloudCredentialID := sbCloudCredentialIDs[0]

	cld := clds.toClouds()[sbCloudCredentialID.CloudID]
	cred := creds.toCloudCredentials()[sbCloudCredentialID.CredentialID]
	k8sConfig, err := getK8sBackendConfig(controller.Name, model.Name, cld, cred)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sb, err := s.getSecretBackend(ctx, tx, secretbackend.BackendIdentifier{Name: kubernetes.BackendName})
	if err != nil {
		return nil, fmt.Errorf("cannot get k8s secret backend for model %q: %w", modelUUID, err)
	}
	sb.Config = k8sConfig.Config
	return sb, nil
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

	var rows secretBackendRows
	err = tx.Query(ctx, stmt, sqlair.M{"identifier": v}).GetAll(&rows)
	if errors.Is(err, sql.ErrNoRows) || len(rows) == 0 {
		return nil, fmt.Errorf("%w: %q", secretbackenderrors.NotFound, v)
	}
	if err != nil {
		return nil, fmt.Errorf("querying secret backends: %w", err)
	}
	return rows.toSecretBackends()[0], nil
}

// GetActiveModelSecretBackend returns the active secret backend ID and config for the given model.
// It returns an error satisfying [modelerrors.NotFound] if the model provided does not exist.
func (s *State) GetActiveModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, *provider.ModelBackendConfig, error) {
	db, err := s.DB()
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	var (
		modelBackend secretbackend.ModelSecretBackend
		backend      *secretbackend.SecretBackend
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelBackend, err = s.getModelSecretBackendDetails(ctx, tx, modelUUID)
		if err != nil {
			return errors.Trace(err)
		}
		if modelBackend.ModelType == coremodel.CAAS && modelBackend.SecretBackendName == kubernetes.BackendName {
			backend, err = s.getK8sSecretBackendForModel(ctx, tx, modelUUID)
		} else {
			backend, err = s.getSecretBackend(ctx, tx, secretbackend.BackendIdentifier{ID: modelBackend.SecretBackendID})
		}
		return errors.Trace(err)
	})
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	return modelBackend.SecretBackendID, &provider.ModelBackendConfig{
		ControllerUUID: modelBackend.ControllerUUID,
		ModelUUID:      modelUUID.String(),
		ModelName:      modelBackend.ModelName,
		BackendConfig: provider.BackendConfig{
			BackendType: backend.BackendType,
			Config:      backend.Config,
		},
	}, nil
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
		var err error
		sb, err = s.getSecretBackend(ctx, tx, params)
		return errors.Trace(err)
	})
	return sb, err
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
	return err
}

// SetModelSecretBackend sets the secret backend for the given model,
// returning an error satisfying [secretbackenderrors.NotFound] if the backend provided does not exist,
// returning an error satisfying [modelerrors.NotFound] if the model provided does not exist.
func (s *State) SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, secretBackendName string) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	backendInfo := ModelSecretBackend{
		ModelID:           modelUUID,
		SecretBackendName: secretBackendName,
	}

	secretBackendSelectQ := `
SELECT b.uuid AS &ModelSecretBackend.secret_backend_uuid
FROM   secret_backend b
WHERE  b.name = $ModelSecretBackend.secret_backend_name
`
	secretBackendSelectStmt, err := s.Prepare(secretBackendSelectQ, backendInfo)
	if err != nil {
		return errors.Trace(err)
	}

	modelBackendUpdate := `
UPDATE model_secret_backend
SET    secret_backend_uuid = $ModelSecretBackend.secret_backend_uuid
WHERE  model_uuid = $ModelSecretBackend.uuid`
	modelBackendUpdateStmt, err := s.Prepare(modelBackendUpdate, backendInfo)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, secretBackendSelectStmt, backendInfo).Get(&backendInfo)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("cannot get secret backend %q: %w", backendInfo.SecretBackendName, secretbackenderrors.NotFound)
		}
		if err != nil {
			return fmt.Errorf("cannot get secret backend %q: %w", backendInfo.SecretBackendName, err)
		}

		var outcome sqlair.Outcome
		err = tx.Query(ctx, modelBackendUpdateStmt, backendInfo).Get(&outcome)
		if err != nil {
			return fmt.Errorf("setting secret backend %q for model %q: %w", backendInfo.SecretBackendName, modelUUID, err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Trace(err)
		}
		if affected == 0 {
			return fmt.Errorf("cannot set secret backend %q for model %q: %w",
				backendInfo.SecretBackendName, backendInfo.ModelID, modelerrors.NotFound,
			)
		}
		return nil
	})
}

// GetSecretBackendReferenceCount returns the number of references to the secret backend.
// It returns 0 if there are no references for the provided secret backend ID.
func (s *State) GetSecretBackendReferenceCount(ctx context.Context, backendID string) (int, error) {
	db, err := s.DB()
	if err != nil {
		return -1, errors.Trace(err)
	}
	input := SecretBackendReference{BackendID: backendID}
	result := Count{}
	stmt, err := s.Prepare(`
SELECT COUNT(*) AS &Count.num
FROM   secret_backend_reference
WHERE  secret_backend_uuid = $SecretBackendReference.secret_backend_uuid`, input, result)
	if err != nil {
		return -1, errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, input).Get(&result)
		return errors.Trace(err)
	})
	if err != nil {
		return -1, fmt.Errorf("cannot get secret backend reference count for %q: %w", backendID, err)
	}
	return result.Num, nil
}

// AddSecretBackendReference adds a reference to the secret backend for the given secret revision, returning an error
// satisfying [secretbackenderrors.NotFound] if the secret backend does not exist,
// or [modelerrors.NotFound] if the model does not exist,
// or [secretbackenderrors.RefCountAlreadyExists] if the reference already exists.
// If the ValueRef is nil, the internal controller backend is used.
// It returns a rollback function which can be used to revert the changes.
func (s *State) AddSecretBackendReference(
	ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string,
) (func() error, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var backendID string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		backendID, err = s.getSecretBackendID(ctx, tx, valueRef)
		if err != nil {
			return errors.Trace(err)
		}
		err := s.addSecretBackendReference(ctx, tx, backendID, modelID, revisionID)
		return errors.Trace(err)
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return func() error {
		err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			input := SecretBackendReference{
				BackendID:        backendID,
				SecretRevisionID: revisionID,
			}
			stmt, err := s.Prepare(`
DELETE FROM secret_backend_reference
WHERE  secret_backend_uuid = $SecretBackendReference.secret_backend_uuid
    AND secret_revision_uuid = $SecretBackendReference.secret_revision_uuid`, input)
			if err != nil {
				return errors.Trace(err)
			}
			err = tx.Query(ctx, stmt, input).Run()
			if err != nil {
				return fmt.Errorf(
					"cannot revert secret backend reference for secret backend %q secret revision %q: %w",
					backendID, revisionID, err,
				)
			}
			return nil
		})
		return errors.Trace(err)
	}, nil
}

func (s *State) getSecretBackendID(ctx context.Context, tx *sqlair.TX, valueRef *secrets.ValueRef) (string, error) {
	arg := secretbackend.BackendIdentifier{}
	if valueRef != nil {
		// We want to check if the backend exists for the given ID.
		arg.ID = valueRef.BackendID
	} else {
		arg.Name = juju.BackendName
	}

	backend, err := s.getSecretBackend(ctx, tx, arg)
	if err != nil {
		return "", fmt.Errorf("cannot get secret backend ID for %q: %w", arg, err)
	}
	return backend.ID, nil
}

func (s *State) addSecretBackendReference(
	ctx context.Context, tx *sqlair.TX, backendID string, modelID coremodel.UUID, revisionID string,
) error {
	ref := SecretBackendReference{
		BackendID:        backendID,
		ModelID:          modelID,
		SecretRevisionID: revisionID,
	}

	stmt, err := s.Prepare(`
INSERT INTO secret_backend_reference (*)
VALUES ($SecretBackendReference.*)`, ref)
	if err != nil {
		return errors.Trace(err)
	}

	err = tx.Query(ctx, stmt, ref).Run()
	if database.IsErrConstraintForeignKey(err) {
		return fmt.Errorf("%w: model %q", modelerrors.NotFound, ref.ModelID)
	}
	if database.IsErrConstraintPrimaryKey(err) {
		return fmt.Errorf(
			"%w: backend %q model %q secret revision %q",
			secretbackenderrors.RefCountAlreadyExists,
			ref.BackendID, ref.ModelID, ref.SecretRevisionID,
		)
	}
	if err != nil {
		return fmt.Errorf("cannot add secret backend reference for secret revision %q: %w", revisionID, err)
	}
	return nil
}

func (s *State) getSecretBackendReferenceBackendID(
	ctx context.Context, tx *sqlair.TX, modelID coremodel.UUID, revisionID string,
) (SecretBackendReference, error) {
	ref := SecretBackendReference{
		ModelID:          modelID,
		SecretRevisionID: revisionID,
	}
	stmt, err := s.Prepare(`
SELECT secret_backend_uuid AS &SecretBackendReference.secret_backend_uuid
FROM   secret_backend_reference
WHERE  model_uuid = $SecretBackendReference.model_uuid
	AND secret_revision_uuid = $SecretBackendReference.secret_revision_uuid`, ref)
	if err != nil {
		return SecretBackendReference{}, errors.Trace(err)
	}

	err = tx.Query(ctx, stmt, ref).Get(&ref)
	if errors.Is(err, sql.ErrNoRows) {
		return SecretBackendReference{}, fmt.Errorf(
			"%w: model %q secret revision %q",
			secretbackenderrors.RefCountNotFound, modelID, revisionID,
		)
	}
	if err != nil {
		return SecretBackendReference{}, fmt.Errorf(
			"cannot get secret backend reference for model %q secret revision %q: %w",
			modelID, revisionID, err,
		)
	}
	return ref, nil
}

// UpdateSecretBackendReference updates the reference to the secret backend for the given secret revision, returning an error
// satisfying [secretbackenderrors.RefCountNotFound] if no existing refcount was found.
// It returns a rollback function which can be used to revert the changes.
func (s *State) UpdateSecretBackendReference(
	ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string,
) (func() error, error) {
	db, err := s.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var existing SecretBackendReference
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		existing, err = s.getSecretBackendReferenceBackendID(ctx, tx, modelID, revisionID)
		if err != nil {
			return errors.Trace(err)
		}

		backendID, err := s.getSecretBackendID(ctx, tx, valueRef)
		if err != nil {
			return errors.Trace(err)
		}
		if err := s.removeSecretBackendReferenceForRevisions(ctx, tx, revisionID); err != nil {
			return errors.Trace(err)
		}
		if err := s.addSecretBackendReference(ctx, tx, backendID, modelID, revisionID); err != nil {
			return errors.Trace(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return func() error {
		err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := s.removeSecretBackendReferenceForRevisions(ctx, tx, revisionID); err != nil {
				return errors.Trace(err)
			}
			err := s.addSecretBackendReference(ctx, tx, existing.BackendID, modelID, revisionID)
			return errors.Trace(err)
		})
		return errors.Trace(err)
	}, nil
}

type secretRevisionIDs []string

// RemoveSecretBackendReference removes the reference to the secret backend for the given secret revisions.
func (s *State) RemoveSecretBackendReference(ctx context.Context, revisionIDs ...string) error {
	if len(revisionIDs) == 0 {
		return nil
	}

	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := s.removeSecretBackendReferenceForRevisions(ctx, tx, revisionIDs...)
		return errors.Trace(err)
	})
	return errors.Trace(err)
}

func (s *State) removeSecretBackendReferenceForRevisions(ctx context.Context, tx *sqlair.TX, revisionIDs ...string) error {
	if len(revisionIDs) == 0 {
		return nil
	}

	input := secretRevisionIDs(revisionIDs)
	stmt, err := s.Prepare(`
DELETE FROM secret_backend_reference
WHERE  secret_revision_uuid IN ($secretRevisionIDs[:])`, input)
	if err != nil {
		return errors.Trace(err)
	}
	err = tx.Query(ctx, stmt, input).Run()
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot remove secret backend reference for %d secret revision(s): %w", len(revisionIDs), err)
	}
	return nil
}

func (s *State) removeSecretBackendReferenceForBackend(ctx context.Context, tx *sqlair.TX, backendID string) error {
	input := SecretBackendReference{BackendID: backendID}
	stmt, err := s.Prepare(`
DELETE FROM secret_backend_reference
WHERE  secret_backend_uuid = $SecretBackendReference.secret_backend_uuid`, input)
	if err != nil {
		return errors.Trace(err)
	}
	err = tx.Query(ctx, stmt, input).Run()
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot remove secret backend reference for secret backend %q: %w", backendID, err)
	}
	return nil
}

// InitialWatchStatementForSecretBackendRotationChanges returns the initial watch statement and the table name to watch for
// secret backend rotation changes.
func (s *State) InitialWatchStatementForSecretBackendRotationChanges() (string, string) {
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

// NamespaceForWatchModelSecretBackend returns the namespace for the model
// secret backend watcher.
func (s *State) NamespaceForWatchModelSecretBackend() string {
	return "model_secret_backend"
}
