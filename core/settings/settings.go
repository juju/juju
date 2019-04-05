// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings

import (
	"fmt"

	"github.com/juju/errors"
)

const (
	added = iota
	modified
	deleted
)

// ItemChange represents the change of an item in a settings.
type ItemChange struct {
	Type     int         `bson:"type"`
	Key      string      `bson:"key"`
	OldValue interface{} `bson:"old,omitempty"`
	NewValue interface{} `bson:"new,omitempty"`
}

func (c *ItemChange) IsAddition() bool {
	return c.Type == added
}

func (c *ItemChange) IsModification() bool {
	return c.Type == modified
}

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

// ApplyDeltaSource turns this second-order delta into a first-older delta
// by replacing the OldValue for any key present in the input ItemChange
// collection.
func (c ItemChanges) ApplyDeltaSource(oldChanges ItemChanges) error {
	m, err := oldChanges.Map()
	if err != nil {
		return errors.Trace(err)
	}

	for i, ch := range c {
		if old, ok := m[ch.Key]; ok {
			c[i].OldValue = old.OldValue
		}
	}
	return nil
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
