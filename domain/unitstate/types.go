// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package unitstate

// UnitState represents the state of the world according to a unit agent at
// hook commit time.
type UnitState struct {
	// Name is the unit name.
	Name string

	// CharmState is key/value pairs for charm attributes.
	CharmState *map[string]string

	// UniterState is the uniter's state as a YAML string.
	UniterState *string

	// RelationState is key/value pairs for relation attributes.
	RelationState *map[int]string

	// StorageState is a YAML string.
	StorageState *string

	// SecretState is a YAML string.
	SecretState *string
}

// RetrievedUnitState represents a unit state persisted and then retrieved
// from the database.
type RetrievedUnitState struct {
	// Name is the unit name.
	Name string

	// CharmState is key/value pairs for charm attributes.
	CharmState map[string]string

	// UniterState is the uniter's state as a YAML string.
	UniterState string

	// RelationState is key/value pairs for relation attributes.
	RelationState map[int]string

	// StorageState is a YAML string.
	StorageState string

	// SecretState is a YAML string.
	SecretState string
}
