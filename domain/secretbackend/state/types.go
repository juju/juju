// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	corecloud "github.com/juju/juju/core/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	secretbackend "github.com/juju/juju/domain/secretbackend"
	backenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/database"
)

// ModelSecretBackend represents a set of data about a model and its secret backend config.
type ModelSecretBackend struct {
	// ID is the unique identifier for the model.
	ID coremodel.UUID `db:"uuid"`
	// Name is the name of the model.
	Name string `db:"name"`
	// Type is the type of the model.
	Type coremodel.ModelType `db:"model_type"`
	// SecretBackendID is the unique identifier for the secret backend configured for the model.
	// TODO: change to string once we changed the `model_secret_backend.secret_backend_uuid` column to be not null.
	SecretBackendID sql.NullString `db:"secret_backend_uuid"`
}

// modelCloudAndCredentialID represents the IDs of a models cloud and cloud
// credential.
type modelCloudAndCredentialID struct {
	CloudID      corecloud.ID      `db:"cloud_uuid"`
	CredentialID corecredential.ID `db:"cloud_credential_uuid"`
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
		return fmt.Errorf("%w: ID is missing", backenderrors.NotValid)
	}
	if p.Name == "" {
		return fmt.Errorf("%w: name is missing", backenderrors.NotValid)
	}
	if p.BackendType == "" {
		return fmt.Errorf("%w: type is missing", backenderrors.NotValid)
	}
	for k, v := range p.Config {
		if k == "" {
			return fmt.Errorf(
				"%w: empty config key for %q", backenderrors.NotValid, p.Name)
		}
		if v == "" {
			return fmt.Errorf(
				"%w: empty config value for %q", backenderrors.NotValid, p.Name)
		}
	}
	return nil
}

// SecretBackend represents a single row from the state database's secret_backend table.
type SecretBackend struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// BackendType is the type of the secret backend.
	BackendType string `db:"backend_type"`
	// TokenRotateInterval is the interval at which the token for the secret backend should be rotated.
	TokenRotateInterval database.NullDuration `db:"token_rotate_interval"`
}

// SecretBackendRotation represents a single row from the state database's secret_backend_rotation table.
type SecretBackendRotation struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"backend_uuid"`
	// NextRotationTime is the time at which the token for the secret backend should be rotated next.
	NextRotationTime sql.NullTime `db:"next_rotation_time"`
}

// SecretBackendConfig represents a single row from the state database's secret_backend_config table.
type SecretBackendConfig struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"backend_uuid"`
	// Name is the name of one record of the secret backend config.
	Name string `db:"name"`
	// Content is the content of the secret backend config.
	Content string `db:"content"`
}

// SecretBackendRow represents a single joined result from secret_backend and secret_backend_config tables.
type SecretBackendRow struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// BackendType is the type of the secret backend.
	BackendType string `db:"backend_type"`
	// TokenRotateInterval is the interval at which the token for the secret backend should be rotated.
	TokenRotateInterval database.NullDuration `db:"token_rotate_interval"`
	// ConfigName is the name of one record of the secret backend config.
	ConfigName string `db:"config_name"`
	// ConfigContent is the content of the secret backend config.
	ConfigContent string `db:"config_content"`
}

// SecretBackendRows represents a slice of SecretBackendRow.
type SecretBackendRows []SecretBackendRow

func (rows SecretBackendRows) toSecretBackends() []*secretbackend.SecretBackend {
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
			currentBackend.Config = make(map[string]string)
		}
		currentBackend.Config[row.ConfigName] = row.ConfigContent
	}
	return result
}

// SecretBackendRotationRow represents a single joined result from secret_backend and secret_backend_rotation tables.
type SecretBackendRotationRow struct {
	// ID is the unique identifier for the secret backend.
	ID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// NextRotationTime is the time at which the token for the secret backend should be rotated next.
	NextRotationTime sql.NullTime `db:"next_rotation_time"`
}

type SecretBackendRotationRows []SecretBackendRotationRow

func (rows SecretBackendRotationRows) toChanges(logger Logger) []watcher.SecretBackendRotateChange {
	var result []watcher.SecretBackendRotateChange
	for _, row := range rows {
		change := watcher.SecretBackendRotateChange{
			ID:   row.ID,
			Name: row.Name,
		}
		next := row.NextRotationTime
		if !next.Valid {
			// This should not happen because it's a NOT NULL field, but log a warning and skip the row.
			logger.Warningf("secret backend %q has no next rotation time", change.ID)
			continue
		}
		change.NextTriggerTime = next.Time
		result = append(result, change)
	}
	return result
}

// ModelCloudCredentialRow represents a single subset of cloud and credential related data from the v_model view.
type ModelCloudCredentialRow struct {
	// CloudName is the name of the cloud.
	CloudName string `db:"cloud_name"`
	// CloudCredentialName is the name of the cloud credential.
	CloudCredentialName string `db:"cloud_credential_name"`
	// OwnerName is the name of the credential owner.
	OwnerName string `db:"owner_name"`
}

// CloudRow represents a single row from the state database's cloud table.
type CloudRow struct {
	// ID holds the cloud document key.
	ID string `db:"uuid"`

	// Name holds the cloud name.
	Name string `db:"name"`

	// Type holds the cloud type reference.
	CloudType string `db:"type"`

	// AuthType is the type of authentication used by the cloud.
	AuthType string `db:"auth_type"`

	// Endpoint holds the cloud's primary endpoint URL.
	Endpoint string `db:"endpoint"`

	// SkipTLSVerify indicates if the client should skip cert validation.
	SkipTLSVerify bool `db:"skip_tls_verify"`

	// ModelName holds the name of the model of the cloud.
	ModelName string `db:"model_name"`

	// ModelOwnerUserName holds the name of the user who owns the model.
	ModelOwnerUserName string `db:"model_owner_user_name"`
}
