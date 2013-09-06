// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
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
//
// TODO(dimitern): This differes from state.Settings.Map() - it does
// not return map[string]interface{}, but since all values are
// expected to be strings anyway, we need to fix the uniter code
// accordingly when migrating to the API.
func (s *Settings) Map() params.Settings {
	settingsCopy := make(params.Settings)
	for k, v := range s.settings {
		settingsCopy[k] = v
	}
	return settingsCopy
}

// Set sets key to value.
//
// TODO(dimitern): value must be a string. Change the code that uses
// this accordingy.
func (s *Settings) Set(key, value string) {
	s.settings[key] = value
	s.deletedKeys.Remove(key)
}

// Delete removes key.
//
// TODO(dimitern) bug=lp:1221798
// Once the machine addressability changes lands, we may need
// to revise the logic here and/or in Write() to take into
// account that the "private-address" setting for a unit can
// be changed outside of the uniter's control. So we may need
// to send diffs of what has changed to make sure we update the
// address (and other settings) correctly, without overwritting.
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
