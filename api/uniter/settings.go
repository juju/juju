// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"github.com/juju/juju/apiserver/params"
)

// This module implements a subset of the interface provided by
// state.Settings, as needed by the uniter API.

// Settings manages changes to unit settings in a relation.
type Settings struct {
	st          *State
	relationTag string
	unitTag     string
	settings    params.Settings
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
	}
}

// Map returns all keys and values of the node.
func (s *Settings) Map() params.Settings {
	settingsCopy := make(params.Settings)
	for k, v := range s.settings {
		if v != "" {
			// Skip deleted keys.
			settingsCopy[k] = v
		}
	}
	return settingsCopy
}

// Set sets key to value.
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
func (s *Settings) Write() error {
	// First make a copy of the map, including deleted keys.
	settingsCopy := make(params.Settings)
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
	// TODO(jam): 2019-07-24 Implement support for UpdateSettings and Application settings.
	//  This might just be UpdateSettings taking an application tag, or we might
	//  want a different API.
	err := s.st.facade.FacadeCall("UpdateSettings", args, &result)
	if err != nil {
		return err
	}
	return result.OneError()
}
