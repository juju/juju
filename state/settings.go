// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sort"

	"github.com/juju/juju/core/settings"
)

// A Settings manages changes to settings as a delta in memory and merges
// them back in the database when explicitly requested.
type Settings struct {
	// cache holds the current values in the config node.
	// The difference between disk and core
	// determines the delta to be applied when Settings.Write
	// is called.
	core map[string]interface{}
}

// Keys returns the current keys in alphabetical order.
func (s *Settings) Keys() []string {
	keys := []string{}
	for key := range s.core {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Map returns all keys and values of the node.
func (s *Settings) Map() map[string]interface{} {
	return nil
}

// Set sets key to value
func (s *Settings) Set(key string, value interface{}) {
	s.core[key] = value
}

// Update sets multiple key/value pairs.
func (s *Settings) Update(kv map[string]interface{}) {
	for key, value := range kv {
		s.core[key] = value
	}
}

// Delete removes key.
func (s *Settings) Delete(key string) {
	delete(s.core, key)
}

// Write writes changes made to c back onto its node.  Changes are written
// as a delta applied on top of the latest version of the node, to prevent
// overwriting unrelated changes made to the node since it was last read.
func (s *Settings) Write() (settings.ItemChanges, error) {
	return nil, nil
}

// Read (re)reads the node data into c.
func (s *Settings) Read() error {
	return nil
}
