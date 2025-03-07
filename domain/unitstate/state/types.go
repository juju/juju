// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "github.com/juju/juju/core/unit"

// unitUUID identifies a unit.
type unitUUID struct {
	// UUID is the universally unique identifier for a unit.
	UUID unit.UUID `db:"uuid"`
}

// unitName identifies a unit.
type unitName struct {
	// Name uniquely identifies a unit and indicates its application.
	// For example, postgresql/3.
	Name unit.Name `db:"name"`
}

// unitState contains a YAML string representing the
// state for a unit's uniter, storage and secrets.
type unitState struct {
	// UniterState is the units uniter state YAML string.
	UniterState string `db:"uniter_state"`
	// StorageState is the units storage state YAML string.
	StorageState string `db:"storage_state"`
	// SecretState is the units secret state YAML string.
	SecretState string `db:"secret_state"`
}

// unitStateVal is a type for holding a key/value pair that is
// a constituent in unit state for charm and relation.
type unitStateKeyVal[T comparable] struct {
	UUID  unit.UUID `db:"unit_uuid"`
	Key   T         `db:"key"`
	Value string    `db:"value"`
}

type unitCharmStateKeyVal unitStateKeyVal[string]
type unitRelationStateKeyVal unitStateKeyVal[int]

func makeUnitCharmStateKeyVals(unitUUID unitUUID, kv map[string]string) []unitCharmStateKeyVal {
	keyVals := make([]unitCharmStateKeyVal, 0, len(kv))
	for k, v := range kv {
		keyVals = append(keyVals, unitCharmStateKeyVal{
			UUID:  unitUUID.UUID,
			Key:   k,
			Value: v,
		})
	}
	return keyVals
}

func makeUnitRelationStateKeyVals(unitUUID unitUUID, kv map[int]string) []unitRelationStateKeyVal {
	keyVals := make([]unitRelationStateKeyVal, 0, len(kv))
	for k, v := range kv {
		keyVals = append(keyVals, unitRelationStateKeyVal{
			UUID:  unitUUID.UUID,
			Key:   k,
			Value: v,
		})
	}
	return keyVals
}

func makeMapFromCharmUnitStateKeyVals(us []unitCharmStateKeyVal) map[string]string {
	m := map[string]string{}
	for _, kv := range us {
		m[kv.Key] = kv.Value
	}
	return m
}

func makeMapFromRelationUnitStateKeyVals(us []unitRelationStateKeyVal) map[int]string {
	m := map[int]string{}
	for _, kv := range us {
		m[kv.Key] = kv.Value
	}
	return m
}
