// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/juju/collections/set"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/secrets/provider/kubernetes"
)

// ModelSecretBackend represents a set of data about a model and its current secret backend config.
type ModelSecretBackend struct {
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

// modelIdentifier represents a set of identifiers for a model.
type modelIdentifier struct {
	// ModelID is the unique identifier for the model.
	ModelID coremodel.UUID `db:"uuid"`
	// ModelName is the name of the model.
	ModelName string `db:"name"`
}

// Validate checks that the model identifier is valid.
func (m modelIdentifier) Validate() error {
	if m.ModelID == "" && m.ModelName == "" {
		return errors.Errorf("both model ID and name are missing")
	}
	return nil
}

// String returns the model name if it is set, otherwise the model ID.
func (m modelIdentifier) String() string {
	if m.ModelName != "" {
		return m.ModelName
	}
	return m.ModelID.String()
}

// modelDetails represents details about a model.
type modelDetails struct {
	// UUID is the unique identifier for the model.
	UUID coremodel.UUID `db:"uuid"`
	// Name is the name of the model.
	Name string `db:"name"`
	// Type is the type of the model.
	Type coremodel.ModelType `db:"model_type"`
}

// controllerName represents details about a controller.
type controllerName struct {
	// Name is the name of the controller.
	Name string `db:"name"`
}

// upsertSecretBackendParams are used to upsert a secret backend.
type upsertSecretBackendParams struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]string
}

// Validate checks that the parameters are valid.
func (p upsertSecretBackendParams) Validate() error {
	if p.ID == "" {
		return errors.Errorf("%w: ID is missing", backenderrors.NotValid)
	}
	if p.Name == "" {
		return errors.Errorf("%w: name is missing", backenderrors.NotValid)
	}
	if p.BackendType == "" {
		return errors.Errorf("%w: type is missing", backenderrors.NotValid)
	}
	for k, v := range p.Config {
		if k == "" {
			return errors.Errorf(
				"%w: empty config key for %q", backenderrors.NotValid, p.Name)

		}
		if v == "" {
			return errors.Errorf(
				"%w: empty config value for %q", backenderrors.NotValid, p.Name)

		}
	}
	return nil
}

// SecretBackend represents a single row from the state database's
// secret_backend table.
type SecretBackend struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// BackendType is the id of the secret backend type.
	BackendTypeID secretbackend.BackendType `db:"backend_type_id"`
	// TokenRotateInterval is the interval at which the token for the secret backend should be rotated.
	TokenRotateInterval database.NullDuration `db:"token_rotate_interval"`
}

// SecretBackendRotation represents a single row from the state database's
// secret_backend_rotation table.
type SecretBackendRotation struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"backend_uuid"`
	// NextRotationTime is the time at which the token for the secret backend
	// should be rotated next.
	NextRotationTime sql.NullTime `db:"next_rotation_time"`
}

// SecretBackendConfig represents a single row from the state database's
// secret_backend_config table.
type SecretBackendConfig struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"backend_uuid"`
	// Name is the name of one record of the secret backend config.
	Name string `db:"name"`
	// Content is the content of the secret backend config.
	Content string `db:"content"`
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

// secretBackendForK8sModelRow represents a single joined result from secret_backend, secret_backend_reference and model tables.
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

func (rows secretBackendForK8sModelRows) toSecretBackend(controllerName string, cldData cloudRows, credData cloudCredentialRows) ([]*secretbackend.SecretBackend, error) {
	clds := cldData.toClouds()
	creds := credData.toCloudCredentials()

	cloudIDs := set.NewStrings()
	var result []*secretbackend.SecretBackend
	for _, row := range rows {
		if cloudIDs.Contains(row.CloudID) {
			continue
		}
		cloudIDs.Add(row.CloudID)
		if _, ok := clds[row.CloudID]; !ok {
			return nil, errors.Errorf("cloud %q not found", row.CloudID)
		}
		if _, ok := creds[row.CredentialID]; !ok {
			return nil, errors.Errorf("cloud credential %q not found", row.CredentialID)
		}
		k8sConfig, err := getK8sBackendConfig(controllerName, row.ModelName, clds[row.CloudID], creds[row.CredentialID])
		if err != nil {
			return nil, errors.Capture(err)
		}
		result = append(result, &secretbackend.SecretBackend{
			ID:          row.ID,
			Name:        kubernetes.BuiltInName(row.ModelName),
			BackendType: row.BackendType,
			NumSecrets:  row.NumSecrets,
			Config:      k8sConfig.Config,
		})

	}
	return result, nil
}

// SecretBackendRotationRow represents a single joined result from
// secret_backend and secret_backend_rotation tables.
type SecretBackendRotationRow struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// NextRotationTime is the time at which the token for the secret backend
	// should be rotated next.
	NextRotationTime sql.NullTime `db:"next_rotation_time"`
}

type SecretBackendRotationRows []SecretBackendRotationRow

func (rows SecretBackendRotationRows) toChanges(logger logger.Logger) []watcher.SecretBackendRotateChange {
	var result []watcher.SecretBackendRotateChange
	for _, row := range rows {
		change := watcher.SecretBackendRotateChange{
			ID:   row.ID,
			Name: row.Name,
		}
		next := row.NextRotationTime
		if !next.Valid {
			// This should not happen because it's a NOT NULL field, but log a
			// warning and skip the row.
			logger.Warningf(context.Background(), "secret backend %q has no next rotation time", change.ID)
			continue
		}
		change.NextTriggerTime = next.Time
		result = append(result, change)
	}
	return result
}

// ModelCloudCredentialRow represents a single subset of cloud and credential
// related data from the v_model view.
type ModelCloudCredentialRow struct {
	// CloudName is the name of the cloud.
	CloudName string `db:"cloud_name"`
	// CloudCredentialName is the name of the cloud credential.
	CloudCredentialName string `db:"cloud_credential_name"`
	// OwnerName is the name of the credential owner.
	OwnerName string `db:"owner_name"`
}

// cloudRow represents a single row from the state database's cloud table.
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
		credentials[id] = cloud.NewNamedCredential(info.Label, cloud.AuthType(info.AuthType), info.Attributes, info.Revoked)
	}
	return credentials
}

// SecretBackendReference represents a single row from the state database's secret_backend_reference table.
type SecretBackendReference struct {
	// BackendID is the unique identifier for the secret backend.
	BackendID string `db:"secret_backend_uuid"`
	// ModelID is the unique identifier for the model.
	ModelID coremodel.UUID `db:"model_uuid"`
	// SecretRevisionID is the unique identifier for the secret revision.
	SecretRevisionID string `db:"secret_revision_uuid"`
}

// Count is a helper struct to count the number of rows.
type Count struct {
	// Num is the number of rows.
	Num int `db:"num"`
}
