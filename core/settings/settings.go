// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package settings

import "fmt"

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

// String returns the item change in a readable format.
func (ic *ItemChange) String() string {
	switch ic.Type {
	case added:
		return fmt.Sprintf("setting added: %v = %v", ic.Key, ic.NewValue)
	case modified:
		return fmt.Sprintf("setting modified: %v = %v (was %v)",
			ic.Key, ic.NewValue, ic.OldValue)
	case deleted:
		return fmt.Sprintf("setting deleted: %v (was %v)", ic.Key, ic.OldValue)
	}
	return fmt.Sprintf("unknown setting change type %d: %v = %v (was %v)", ic.Type, ic.Key, ic.NewValue, ic.OldValue)
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

func (ics ItemChanges) Len() int           { return len(ics) }
func (ics ItemChanges) Less(i, j int) bool { return ics[i].Key < ics[j].Key }
func (ics ItemChanges) Swap(i, j int)      { ics[i], ics[j] = ics[j], ics[i] }
