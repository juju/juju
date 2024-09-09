// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

// unitUUID identifies a unit.
type unitUUID struct {
	// UUID is the universally unique identifier for a unit.
	UUID string `db:"uuid"`
}

// unitName identifies a unit.
type unitName struct {
	// Name uniquely identifies a unit and indicates its application.
	// For example, postgresql/3.
	Name string `db:"name"`
}

// unitState contains a YAML string representing the
// state for a unit's uniter, storage or secrets.
type unitState struct {
	// State is the YAML string.
	State string `db:"state"`
}

// unitStateVal is a type for holding a key/value pair that is
// a constituent in unit state for charm and relation.
type unitStateKeyVal struct {
	UUID string `db:"unit_uuid"`
	// TODO (manadart 2024-09-09): This should be a generic T congruent with
	// the function below. However, at the time of writing, SQLair does not
	// support generic argumentation.
	Key   any    `db:"key"`
	Value string `db:"value"`
}

func makeUnitStateKeyVals[T comparable](unitUUID string, kv map[T]string) []unitStateKeyVal {
	keyVals := make([]unitStateKeyVal, 0, len(kv))
	for k, v := range kv {
		keyVals = append(keyVals, unitStateKeyVal{
			UUID:  unitUUID,
			Key:   k,
			Value: v,
		})
	}
	return keyVals
}
