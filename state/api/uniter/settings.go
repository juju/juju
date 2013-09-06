// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/utils/set"
)

// This module implements a subset of the interface provided by
// state.Settings, as needed by the uniter API.

// Settings manages changes to unit settings in a relation.
type Settings struct {
	st          *State
	relationTag string
	unitTag     string
	settings    params.Settings
	deletedKeys set.Strings
}

func newSettings(st *State, relationTag, unitTag string, settings params.Settings) *Settings {
	if settings == nil {
		settings = make(params.Settings)
	}
	return &Settings{
		st:          st,
		relationTag: relationTag,
		unitTag:     unitTag,
		settings:    settings,
		deletedKeys: set.NewStrings(),
	}
}

// Map returns all keys and values of the node.
func (s *Settings) Map() map[string]interface{} {
	// Code expects map[string]interface{}, even
	// though the actual settings are always strings,
	// so we need to convert them.
	result := make(map[string]interface{})
	for k, v := range s.settings {
		result[k] = v
	}
	return result
}

// Set sets key to value.
// TODO: value must be a string. Change the code accordingy.
func (s *Settings) Set(key string, value interface{}) {
	stringValue, ok := value.(string)
	if !ok {
		panic(fmt.Sprintf("cannot set non-string value %v for setting %q", value, key))
	}
	s.settings[key] = stringValue
	s.deletedKeys.Remove(key)
}

// Delete removes key.
func (s *Settings) Delete(key string) {
	s.deletedKeys.Add(key)
	delete(s.settings, key)
}

// Write writes changes made to s back onto its node. Keys set to
// empty values will be deleted, others will be updated to the new
// value.
func (s *Settings) Write() error {
	// First make a copy of the map.
	settingsCopy := make(params.Settings)
	for k, v := range s.settings {
		settingsCopy[k] = v
	}
	// Mark removed keys for deletion.
	for _, k := range s.deletedKeys.Values() {
		settingsCopy[k] = ""
	}

	var result params.ErrorResults
	args := params.RelationUnitsSettings{
		RelationUnits: []params.RelationUnitSettings{{
			Relation: s.relationTag,
			Unit:     s.unitTag,
			Settings: settingsCopy,
		}},
	}
	err := s.st.caller.Call("Uniter", "", "UpdateSettings", args, &result)
	if err != nil {
		return err
	}
	err = result.OneError()
	if err == nil {
		s.deletedKeys = set.NewStrings()
	}
	return err
}
