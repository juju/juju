// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitstate

// AgentState represents the state of the world according
// to a unit agent at hook commit time.
type AgentState struct {
	// Name is the unit name.
	Name string

	// CharmState is key/value pairs for charm attributes.
	CharmState *map[string]string

	// UniterState is the uniter's state as a YAML string.
	UniterState *string

	// RelationState is key/values pairs for relation attributes.
	RelationState *map[int]string

	// StorageState is a YAML string.
	StorageState *string

	// SecretState is a YAML string.
	SecretState *string
}
