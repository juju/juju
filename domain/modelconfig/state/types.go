// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// dbKey represents the key column from a model_config row.
// Once SQLair supports scalar types the key can be selected directly into a
// string and this struct will no longer be needed.
type dbKey struct {
	Key string `db:"key"`
}

// dbAgentVersion represents the target agent version and stream for the model.
type dbAgentVersionAndStream struct {
	Stream             string `db:"name"`
	TargetAgentVersion string `db:"target_version"`
}

// dbKeyValue represents a key-value pair from the model_config table.
type dbKeyValue struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

// dbKeys represents a slice of keys from the model_config table.
type dbKeys []string

// dbSpace represents the name column from the space table.
type dbSpace struct {
	Space string `db:"name"`
}
