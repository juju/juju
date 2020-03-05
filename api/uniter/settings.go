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
	dirty       bool
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
	s.dirty = true
}

// Delete removes key.
func (s *Settings) Delete(key string) {
	// Keys are only marked as deleted, because we need to report them
	// back to the server for deletion on Write().
	s.settings[key] = ""
	s.dirty = true
}

// FinalResult returns a params.Settings with the final updates applied.
// This includes entries that were deleted.
func (s *Settings) FinalResult() params.Settings {
	// First make a copy of the map, including deleted keys.
	settingsCopy := make(params.Settings)
	for k, v := range s.settings {
		settingsCopy[k] = v
	}
	return settingsCopy
}

func (s *Settings) IsDirty() bool {
	return s.dirty
}
