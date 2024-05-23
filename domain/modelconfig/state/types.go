// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import coremodel "github.com/juju/juju/core/model"

// dbKey represents the key column from a model_config row.
// Once SQLair supports scalar types the key can be selected directly into a
// string and this struct will no longer be needed.
type dbKey struct {
	Key string `db:"key"`
}

// dbAgentVersion represents the target agent version from the model table.
type dbAgentVersion struct {
	TargetAgentVersion string `db:"target_agent_version"`
}

// dbKeyValue represents a key-value pair from the model_config table.
type dbKeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// dbKeys represents a slice of keys from the model_config table.
type dbKeys []string

// ModelInfo holds model metadata.
type ModelInfo struct {
	UUID coremodel.UUID      `db:"uuid"`
	Type coremodel.ModelType `db:"type"`
}

// SecretBackendInfo represents a secret backend.
type SecretBackendInfo struct {
	BackendUUID string `db:"uuid"`
	Name        string `db:"name"`
	ModelUUID   string `db:"model_uuid"`
}
