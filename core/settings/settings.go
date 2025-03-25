// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings

import (
	"fmt"

	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

const (
	added = iota
	modified
	deleted
)

// ItemChange represents the change of an item in a settings collection.
type ItemChange struct {
	// Type is an enumeration indicating the type of settings change.
	Type int
	// Key is the setting being changed.
	Key string
	// OldValue is the previous value for the setting.
	OldValue interface{}
	// NewValue is the settings value resulting from this change.
	NewValue interface{}
}

// IsAddition returns true if this change indicates a settings value
// not previously defines.
func (c *ItemChange) IsAddition() bool {
	return c.Type == added
}

// IsModification returns true if this change is an update of a previously
// operator-defined setting.
func (c *ItemChange) IsModification() bool {
	return c.Type == modified
}

// IsDeletion returns true if this change indicates the removal of a
// previously operator-defined setting.
func (c *ItemChange) IsDeletion() bool {
	return c.Type == deleted
}

// String returns the item change in a readable format.
func (c *ItemChange) String() string {
	switch c.Type {
	case added:
		return fmt.Sprintf("setting added: %v = %v", c.Key, c.NewValue)
	case modified:
		return fmt.Sprintf("setting modified: %v = %v (was %v)",
			c.Key, c.NewValue, c.OldValue)
	case deleted:
		return fmt.Sprintf("setting deleted: %v (was %v)", c.Key, c.OldValue)
	}
	return fmt.Sprintf("unknown setting change type %d: %v = %v (was %v)", c.Type, c.Key, c.NewValue, c.OldValue)
}

// MakeAddition returns an itemChange indicating a modification of the input
// key, with its new value.
func MakeAddition(key string, newVal interface{}) ItemChange {
	return ItemChange{
		Type:     added,
		Key:      key,
		NewValue: newVal,
	}
}

// MakeModification returns an ItemChange indicating a modification of the
// input key, with its old and new values.
func MakeModification(key string, oldVal, newVal interface{}) ItemChange {
	return ItemChange{
		Type:     modified,
		Key:      key,
		OldValue: oldVal,
		NewValue: newVal,
	}
}

// MakeDeletion returns an ItemChange indicating a deletion of the input key,
// with its old value.
func MakeDeletion(key string, oldVal interface{}) ItemChange {
	return ItemChange{
		Type:     deleted,
		Key:      key,
		OldValue: oldVal,
	}
}

// ItemChanges contains a slice of item changes in a config node.
// It implements the sort interface to sort the items changes by key.
type ItemChanges []ItemChange

// ApplyDeltaSource uses this second-order delta to generate a first-older
// delta.
// It accepts a collection of changes representing a previous state.
// These are combined with the current changes to generate a new collection.
// It addresses a requirement that each branch change should represent the
// "from" state of master config at the time it is first created.
func (c ItemChanges) ApplyDeltaSource(oldChanges ItemChanges) (ItemChanges, error) {
	m, err := oldChanges.Map()
	if err != nil {
		return nil, errors.Capture(err)
	}

	res := make(ItemChanges, len(c))
	copy(res, c)

	for i, ch := range res {
		if old, ok := m[ch.Key]; ok {
			switch {
			case old.OldValue == nil && ch.IsModification():
				// Any previous change with no old value indicates that the key
				// was not defined in settings when the branch was created.
				// Indicate the modification as an addition.
				res[i] = MakeAddition(ch.Key, ch.NewValue)
			case old.OldValue != nil && c[i].IsAddition():
				// If a setting that existed at branch creation time has been
				// deleted and re-added, indicate it as a modification.
				res[i] = MakeModification(ch.Key, old.OldValue, ch.NewValue)
			default:
				// Preserve all old values.
				res[i].OldValue = old.OldValue
			}

			// Remove the map entry to indicate we have dealt with the key.
			delete(m, ch.Key)
		}
	}

	// If there is an old change not present in this collection,
	// then we know that the setting was reset to its original value.
	// If this setting is subsequently reinstated, it is possible to lose the
	// original "from" value if master is updated in the meantime.
	// So we maintain an entry with the same to/from in order to retain the old
	// value from when the configuration setting was first touched.
	for key, old := range m {
		res = append(res, MakeModification(key, old.OldValue, old.OldValue))
	}

	return res, nil
}

// EffectiveChanges returns the effective changes resulting from the
// application of these changes to the input defaults.
func (c ItemChanges) EffectiveChanges(defaults charm.Settings) map[string]interface{} {
	result := make(map[string]interface{})
	for _, change := range c {
		key := change.Key

		switch {
		case change.IsDeletion():
			result[key] = defaults[key]
		default:
			result[key] = change.NewValue
		}
	}
	return result
}

// Map is a convenience method for working with collections of changes.
// It returns a map representation of the change collection,
// indexed with the change key.
// An error return indicates that the collection had duplicate keys.
func (c ItemChanges) Map() (map[string]ItemChange, error) {
	m := make(map[string]ItemChange, len(c))
	for _, ch := range c {
		k := ch.Key
		if _, ok := m[k]; ok {
			return nil, errors.Errorf("duplicated key in settings collection: %q", k)
		}
		m[k] = ch
	}
	return m, nil
}

func (c ItemChanges) Len() int           { return len(c) }
func (c ItemChanges) Less(i, j int) bool { return c[i].Key < c[j].Key }
func (c ItemChanges) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
