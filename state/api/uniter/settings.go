// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// This module implements a subset of the interface provided by
// state.Settings, as needed by the uniter API.

// TODO: Only the required calls are added as placeholders,
// the actual implementation will come in a follow-up.

// Settings manages changes to unit settings in a relation.
type Settings struct {
	st *State
	// TODO: Add fields.
}

// Map returns all keys and values of the node.
func (s *Settings) Map() map[string]interface{} {
	panic("not implemented")
}

// Set sets key to value.
// TODO: value must be a string. Change the code accordingy.
func (s *Settings) Set(key string, value interface{}) {
	panic("not implemented")
}

// Delete removes key.
func (s *Settings) Delete(key string) {
	panic("not implemented")
}

// Write writes changes made to s back onto its node. Keys set to
// empty values will be deleted, others will be updated to the new
// value.
func (s *Settings) Write() error {
	// TODO: Call Uniter.UpdateSettings()
	// The original state.Settings.Write() returns []ItemChange,
	// but since it's not used by the uniter, this call just
	// returns an error.
	panic("not implemented")
}
