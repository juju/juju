// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"launchpad.net/juju-core/state/api/params"
)

// This module implements a subset of the interface provided by
// state.Settings, as needed by the uniter API.

// Settings manages changes to unit settings in a relation.
type Settings struct {
	st          *State
	relationTag string
	unitTag     string
	settings    params.RelationSettings
}

func newSettings(st *State, relationTag, unitTag string, settings params.RelationSettings) *Settings {
	if settings == nil {
		settings = make(params.RelationSettings)
	}
	return &Settings{
		st:          st,
		relationTag: relationTag,
		unitTag:     unitTag,
		settings:    settings,
	}
}

// Map returns all keys and values of the node.
//
// TODO(dimitern): This differes from state.Settings.Map() - it does
// not return map[string]interface{}, but since all values are
// expected to be strings anyway, we need to fix the uniter code
// accordingly when migrating to the API.
func (s *Settings) Map() params.RelationSettings {
	settingsCopy := make(params.RelationSettings)
	for k, v := range s.settings {
		if v != "" {
			// Skip deleted keys.
			settingsCopy[k] = v
		}
	}
	return settingsCopy
}

// Set sets key to value.
//
// TODO(dimitern): value must be a string. Change the code that uses
// this accordingly.
func (s *Settings) Set(key, value string) {
	s.settings[key] = value
}

// Delete removes key.
func (s *Settings) Delete(key string) {
	// Keys are only marked as deleted, because we need to report them
	// back to the server for deletion on Write().
	s.settings[key] = ""
}

// Write writes changes made to s back onto its node. Keys set to
// empty values will be deleted, others will be updated to the new
// value.
//
// TODO(dimitern): 2013-09-06 bug 1221798
// Once the machine addressability changes lands, we may need to
// revise the logic here to take into account that the
// "private-address" setting for a unit can be changed outside of the
// uniter's control. So we may need to send diffs of what has changed
// to make sure we update the address (and other settings) correctly,
// without overwritting.
func (s *Settings) Write() error {
	// First make a copy of the map, including deleted keys.
	settingsCopy := make(params.RelationSettings)
	for k, v := range s.settings {
		settingsCopy[k] = v
	}

	var result params.ErrorResults
	args := params.RelationUnitsSettings{
		RelationUnits: []params.RelationUnitSettings{{
			Relation: s.relationTag,
			Unit:     s.unitTag,
			Settings: settingsCopy,
		}},
	}
	err := s.st.call("UpdateSettings", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
