// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"sort"

	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain"
)

// Model represents a single subset of a row from the state database's model_metadata table.
type Model struct {
	// UUID is the unique identifier for the model.
	UUID string `db:"uuid"`
	// Name is the name of the model.
	Name string `db:"name"`
	// Type is the type of the model.
	Type model.ModelType `db:"type"`
}

// SecretBackend represents a single row from the state database's secret_backend table.
type SecretBackend struct {
	// UUID is the unique identifier for the secret backend.
	UUID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// Type is the type of the secret backend.
	BackendType string `db:"backend_type"`
	// TokenRotateInterval is the interval at which the token for the secret backend should be rotated.
	TokenRotateInterval domain.NullableDuration `db:"token_rotate_interval"`
}

// SecretBackendRotation represents a single row from the state database's secret_backend_rotation table.
type SecretBackendRotation struct {
	// BackendUUID is the unique identifier for the secret backend.
	BackendUUID string `db:"backend_uuid"`
	// NextRotationTime is the time at which the token for the secret backend should be rotated next.
	NextRotationTime sql.NullTime `db:"next_rotation_time"`
}

// SecretBackendConfig represents a single row from the state database's secret_backend_config table.
type SecretBackendConfig struct {
	// BackendUUID is the unique identifier for the secret backend.
	BackendUUID string `db:"backend_uuid"`
	// Name is the name of one record of the secret backend config.
	Name string `db:"name"`
	// Content is the content of the secret backend config.
	Content string `db:"content"`
}

// SecretBackendRow represents a single joined result from secret_backend and secret_backend_config tables.
type SecretBackendRow struct {
	// UUID is the unique identifier for the secret backend.
	UUID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// Type is the type of the secret backend.
	BackendType string `db:"backend_type"`
	// TokenRotateInterval is the interval at which the token for the secret backend should be rotated.
	TokenRotateInterval domain.NullableDuration `db:"token_rotate_interval"`
	// ConfigName is the name of one record of the secret backend config.
	ConfigName string `db:"config_name"`
	// ConfigContent is the content of the secret backend config.
	ConfigContent string `db:"config_content"`
}

// SecretBackendRows represents a slice of SecretBackendRow.
type SecretBackendRows []SecretBackendRow

// ToSecretBackendInfo returns a slice of coresecrets.SecretBackend from the rows.
func (rows SecretBackendRows) ToSecretBackendInfo() []*coresecrets.SecretBackend {
	// Sort the rows by backend name to ensure that we group the config.
	sort.Slice(rows, func(i, j int) bool {
		return rows[i].Name < rows[j].Name
	})
	var result []*coresecrets.SecretBackend
	var currentBackend *coresecrets.SecretBackend
	for _, row := range rows {
		backend := coresecrets.SecretBackend{
			ID:          row.UUID,
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
			currentBackend.Config = make(map[string]interface{})
		}
		currentBackend.Config[row.ConfigName] = row.ConfigContent
	}
	return result
}

// SecretBackendRotationRow represents a single joined result from secret_backend and secret_backend_rotation tables.
type SecretBackendRotationRow struct {
	// UUID is the unique identifier for the secret backend.
	UUID string `db:"uuid"`
	// Name is the name of the secret backend.
	Name string `db:"name"`
	// NextRotationTime is the time at which the token for the secret backend should be rotated next.
	NextRotationTime sql.NullTime `db:"next_rotation_time"`
}

type SecretBackendRotationRows []SecretBackendRotationRow

// ToChanges returns a slice of watcher.SecretBackendRotateChange from the rows.
func (rows SecretBackendRotationRows) ToChanges(logger Logger) []watcher.SecretBackendRotateChange {
	var result []watcher.SecretBackendRotateChange
	for _, row := range rows {
		change := watcher.SecretBackendRotateChange{
			ID:   row.UUID,
			Name: row.Name,
		}
		next := row.NextRotationTime
		if !next.Valid {
			logger.Warningf("secret backend %q has no next rotation time", change.ID)
			continue
		}
		change.NextTriggerTime = next.Time
		result = append(result, change)
	}
	return result
}
