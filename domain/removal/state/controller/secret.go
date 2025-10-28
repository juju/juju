// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"fmt"
	"sort"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/cloud"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/credential"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// TODO (manadart 2025-10-25): The logic here is duplicated from the
// secretbackend domain. This duplication is excessive.
// This should work in a similar fashion to cloud providers, i.e. a secret
// backend dependency should be passed into the services that require it,
// and that dependency should be backed by its own domain.
// At that time, a lot of what is here, which does not need to be transactional,
// should be composed in the service layer.
// At that time, take the logic from here rather than the secretbackend domain
// because this better reflects established conventions.
// Types are located in this file for ease of transplant when that activity is
// undertaken.
// Tests for this functionality are not duplicated from secretbackend.

// GetActiveModelSecretBackend returns the active secret backend ID and config
// for the given model.
// It returns an error satisfying [modelerrors.NotFound] if the model provided
// does not exist.
func (s *State) GetActiveModelSecretBackend(
	ctx context.Context, modelUUID string,
) (string, *provider.ModelBackendConfig, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return "", nil, errors.Capture(err)
	}
	var (
		modelBackend modelSecretBackend
		backend      *secretbackend.SecretBackend
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		modelBackend, err = s.getModelSecretBackendDetails(ctx, tx, modelUUID)
		if err != nil {
			return errors.Capture(err)
		}
		if modelBackend.ModelType == coremodel.CAAS && modelBackend.SecretBackendName == kubernetes.BackendName {
			backend, err = s.getK8sSecretBackendForModel(ctx, tx, modelUUID)
		} else {
			backend, err = s.getSecretBackend(
				ctx, tx, secretbackend.BackendIdentifier{ID: modelBackend.SecretBackendID})
		}
		return errors.Capture(err)
	})
	if err != nil {
		return "", nil, errors.Capture(err)
	}
	return modelBackend.SecretBackendID, &provider.ModelBackendConfig{
		ControllerUUID: modelBackend.ControllerUUID,
		ModelUUID:      modelUUID,
		ModelName:      modelBackend.ModelName,
		BackendConfig: provider.BackendConfig{
			BackendType: backend.BackendType,
			Config:      backend.Config,
		},
	}, nil
}

func (s *State) getModelSecretBackendDetails(
	ctx context.Context, tx *sqlair.TX, mUUID string,
) (modelSecretBackend, error) {
	input := entityUUID{UUID: mUUID}
	var backend modelSecretBackend

	stmt, err := s.Prepare(`
SELECT &modelSecretBackend.*
FROM   v_model_secret_backend
WHERE  uuid = $entityUUID.uuid`, input, backend)
	if err != nil {
		return backend, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, input).Get(&backend)
	if errors.Is(err, sql.ErrNoRows) {
		return backend, errors.Errorf(
			"getting secret backend for model %q: %w", mUUID, modelerrors.NotFound)
	}

	return backend, errors.Capture(err)
}

func (s *State) getK8sSecretBackendForModel(
	ctx context.Context, tx *sqlair.TX, mUUID string,
) (*secretbackend.SecretBackend, error) {
	modelUUID := entityUUID{UUID: mUUID}

	q := `
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
    JOIN v_cloud_credential_attribute vcca ON vm.cloud_credential_uuid = vcca.uuid
WHERE vm.uuid = $entityUUID.uuid
GROUP BY vm.name, vcca.attribute_key`

	stmt, err := s.Prepare(
		q, modelUUID, modelDetails{}, secretBackendForK8sModelRow{}, cloudRow{}, cloudCredentialRow{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Controller name is still stored in controller config.
	var controller controllerName
	controllerNameStmt, err := s.Prepare(
		"SELECT value AS &controllerName.name FROM v_controller_config WHERE key = 'controller-name'", controller)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var models []modelDetails
	var sbCloudCredentialIDs secretBackendForK8sModelRows
	var clds cloudRows
	var creds cloudCredentialRows
	err = tx.Query(ctx, stmt, modelUUID).GetAll(&models, &sbCloudCredentialIDs, &clds, &creds)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("%w: %q", modelerrors.NotFound, mUUID)
	}
	if err != nil {
		return nil, errors.Errorf("fetching k8s secret backend credential for model %q: %w", mUUID, err)
	}
	if len(models) == 0 || len(sbCloudCredentialIDs) == 0 {
		return nil, errors.Errorf("%w: %q", modelerrors.NotFound, mUUID)
	}

	err = tx.Query(ctx, controllerNameStmt).Get(&controller)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("controller name key not found")
	}
	if err != nil {
		return nil, errors.Errorf("cannot select controller name")
	}
	model := models[0]
	if model.Type != coremodel.CAAS {
		return nil, errors.Errorf("%w: %q", modelerrors.NotFound, mUUID)
	}
	sbCloudCredentialID := sbCloudCredentialIDs[0]

	cld := clds.toClouds()[sbCloudCredentialID.CloudID]
	cred := creds.toCloudCredentials()[sbCloudCredentialID.CredentialID]
	k8sConfig, err := getK8sBackendConfig(controller.Name, model.Name, cld, cred)
	if err != nil {
		return nil, errors.Capture(err)
	}

	sb, err := s.getSecretBackend(ctx, tx, secretbackend.BackendIdentifier{Name: kubernetes.BackendName})
	if err != nil {
		return nil, errors.Errorf("getting k8s secret backend for model %q: %w", mUUID, err)
	}
	sb.Config = k8sConfig.Config
	return sb, nil
}

func getK8sBackendConfig(
	controllerName, modelName string, cloud cloud.Cloud, cred cloud.Credential,
) (*provider.BackendConfig, error) {
	spec, err := cloudspec.MakeCloudSpec(cloud, "", &cred)
	if err != nil {
		return nil, errors.Capture(err)
	}
	k8sConfig, err := kubernetes.BuiltInConfig(controllerName, modelName, spec)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return k8sConfig, nil
}

func (s *State) getSecretBackend(
	ctx context.Context, tx *sqlair.TX, identifier secretbackend.BackendIdentifier,
) (*secretbackend.SecretBackend, error) {
	if identifier.ID == "" && identifier.Name == "" {
		return nil, errors.Errorf("%w: both ID and name are missing", secretbackenderrors.NotValid)
	}
	if identifier.ID != "" && identifier.Name != "" {
		return nil, errors.Errorf("%w: both ID and name are provided", secretbackenderrors.NotValid)
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
		return nil, errors.Capture(err)
	}

	var rows secretBackendRows
	err = tx.Query(ctx, stmt, sqlair.M{"identifier": v}).GetAll(&rows)
	if errors.Is(err, sql.ErrNoRows) || len(rows) == 0 {
		return nil, errors.Errorf("%w: %q", secretbackenderrors.NotFound, v)
	}
	if err != nil {
		return nil, errors.Errorf("querying secret backends: %w", err)
	}
	return rows.toSecretBackends()[0], nil
}

type modelDetails struct {
	// UUID is the unique identifier for the model.
	UUID coremodel.UUID `db:"uuid"`

	// Name is the name of the model.
	Name string `db:"name"`

	// Type is the type of the model.
	Type coremodel.ModelType `db:"model_type"`
}

type modelSecretBackend struct {
	// ControllerUUID is the UUID of the controller.
	ControllerUUID string `db:"controller_uuid"`

	// ModelID is the unique identifier for the model.
	ModelID coremodel.UUID `db:"uuid"`

	// ModelName is the name of the model.
	ModelName string `db:"name"`

	// ModelType is the type of the model.
	ModelType coremodel.ModelType `db:"model_type"`

	// SecretBackendID is the unique identifier for the secret backend configured for the model.
	SecretBackendID string `db:"secret_backend_uuid"`

	// SecretBackendName is the name of the secret backend configured for the model.
	SecretBackendName string `db:"secret_backend_name"`
}

// SecretBackendRow represents a single joined result from secret_backend and
// secret_backend_config tables.
type SecretBackendRow struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"uuid"`

	// Name is the name of the secret backend.
	Name string `db:"name"`

	// BackendType is the type of the secret backend.
	BackendType string `db:"backend_type"`

	// TokenRotateInterval is the interval at which the token for the secret
	// backend should be rotated.
	TokenRotateInterval database.NullDuration `db:"token_rotate_interval"`

	// ConfigName is the name of one record of the secret backend config.
	ConfigName string `db:"config_name"`

	// ConfigContent is the content of the secret backend config.
	ConfigContent string `db:"config_content"`

	// NumSecrets is the number of secrets stored in the secret backend.
	NumSecrets int `db:"num_secrets"`
}

// secretBackendRows represents a slice of SecretBackendRow.
type secretBackendRows []SecretBackendRow

func (rows secretBackendRows) toSecretBackends() []*secretbackend.SecretBackend {
	// Sort the rows by backend name to ensure that we group the config.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})
	var result []*secretbackend.SecretBackend
	var currentBackend *secretbackend.SecretBackend
	for _, row := range rows {
		backend := secretbackend.SecretBackend{
			ID:          row.ID,
			Name:        row.Name,
			BackendType: row.BackendType,
			NumSecrets:  row.NumSecrets,
		}
		interval := row.TokenRotateInterval
		if interval.Valid {
			backend.TokenRotateInterval = &interval.Duration
		}

		if currentBackend == nil || currentBackend.ID != backend.ID {
			// Encountered a new backend.
			currentBackend = &backend
			result = append(result, currentBackend)
		}
		if row.ConfigName == "" || row.ConfigContent == "" {
			// No config for this row.
			continue
		}

		if currentBackend.Config == nil {
			currentBackend.Config = make(map[string]any)
		}
		currentBackend.Config[row.ConfigName] = row.ConfigContent
	}
	return result
}

// secretBackendForK8sModelRow represents a single joined result from
// secret_backend, secret_backend_reference and model tables.
type secretBackendForK8sModelRow struct {
	SecretBackendRow

	// ModelName is the name of the model.
	ModelName string `db:"model_name"`

	// CloudID is the cloud UUID.
	CloudID string `db:"cloud_uuid"`

	// CredentialID is the cloud credential UUID.
	CredentialID string `db:"cloud_credential_uuid"`
}

type secretBackendForK8sModelRows []secretBackendForK8sModelRow

// cloudRow represents a single row from the cloud table.
type cloudRow struct {
	// ID holds the cloud UUID.
	ID string `db:"uuid"`

	// Name holds the cloud name.
	Name string `db:"name"`

	// Endpoint holds the cloud's primary endpoint URL.
	Endpoint string `db:"endpoint"`

	// SkipTLSVerify indicates if the client should skip cert validation.
	SkipTLSVerify bool `db:"skip_tls_verify"`

	// IsControllerCloud indicates if the cloud is hosting the controller model.
	IsControllerCloud bool `db:"is_controller_cloud"`

	// CACert holds the ca cert.
	CACert string `db:"ca_cert"`
}

func (r cloudRow) toCloud() cloud.Cloud {
	return cloud.Cloud{
		Name:              r.Name,
		Endpoint:          r.Endpoint,
		SkipTLSVerify:     r.SkipTLSVerify,
		IsControllerCloud: r.IsControllerCloud,
		CACertificates:    []string{r.CACert},
	}
}

type cloudRows []cloudRow

func (rows cloudRows) toClouds() map[string]cloud.Cloud {
	clouds := make(map[string]cloud.Cloud, len(rows))
	for _, row := range rows {
		clouds[row.ID] = row.toCloud()
	}
	return clouds
}

type cloudCredentialRow struct {
	// ID holds the cloud credential UUID.
	ID string `db:"uuid"`

	// Name holds the cloud credential name.
	Name string `db:"name"`

	// AuthType holds the cloud credential auth type.
	AuthType string `db:"auth_type"`

	// Revoked is true if the credential has been revoked.
	Revoked bool `db:"revoked"`

	// AttributeKey contains a single credential attribute key
	AttributeKey string `db:"attribute_key"`

	// AttributeValue contains a single credential attribute value
	AttributeValue string `db:"attribute_value"`
}

type cloudCredentialRows []cloudCredentialRow

func (rows cloudCredentialRows) toCloudCredentials() map[string]cloud.Credential {
	credentials := make(map[string]cloud.Credential, len(rows))
	data := make(map[string]credential.CloudCredentialInfo, len(rows))
	for _, row := range rows {
		if _, ok := data[row.ID]; !ok {
			data[row.ID] = credential.CloudCredentialInfo{
				Label:      row.Name,
				AuthType:   row.AuthType,
				Revoked:    row.Revoked,
				Attributes: make(map[string]string),
			}
		}
		if row.AttributeKey != "" {
			data[row.ID].Attributes[row.AttributeKey] = row.AttributeValue
		}
	}
	for id, info := range data {
		credentials[id] = cloud.NewNamedCredential(
			info.Label, cloud.AuthType(info.AuthType), info.Attributes, info.Revoked)
	}
	return credentials
}

type controllerName struct {
	// Name is the name of the controller.
	Name string `db:"name"`
}
