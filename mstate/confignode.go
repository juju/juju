package mstate

import (
	"fmt"
	"sort"
)

const (
	ItemAdded = iota
	ItemModified
	ItemDeleted
)

// ItemChange represents the change of an item in a configNode.
type ItemChange struct {
	Type     int
	Key      string
	OldValue interface{}
	NewValue interface{}
}

// String returns the item change in a readable format.
func (ic *ItemChange) String() string {
	switch ic.Type {
	case ItemAdded:
		return fmt.Sprintf("setting added: %v = %v", ic.Key, ic.NewValue)
	case ItemModified:
		return fmt.Sprintf("setting modified: %v = %v (was %v)",
			ic.Key, ic.NewValue, ic.OldValue)
	case ItemDeleted:
		return fmt.Sprintf("setting deleted: %v (was %v)", ic.Key, ic.OldValue)
	}
	return fmt.Sprintf("unknown setting change type %d: %v = %v (was %v)",
		ic.Type, ic.Key, ic.NewValue, ic.OldValue)
}

// itemChangeSlice contains a slice of item changes in a config node.
// It implements the sort interface to sort the items changes by key.
type itemChangeSlice []ItemChange

func (ics itemChangeSlice) Len() int           { return len(ics) }
func (ics itemChangeSlice) Less(i, j int) bool { return ics[i].Key < ics[j].Key }
func (ics itemChangeSlice) Swap(i, j int)      { ics[i], ics[j] = ics[j], ics[i] }

// A ConfigNode represents the data of a ZooKeeper node
// containing YAML-based settings. It manages changes to
// the data as a delta in memory, and merges them back
// onto the node when explicitly requested.
type ConfigNode struct {
	st   *State
	path string
	// pristineCache holds the values in the config node before
	// any keys have been changed. It is reset on Read and Write
	// operations.
	pristineCache map[string]interface{}
	// cache holds the current values in the config node.
	// The difference between pristineCache and cache
	// determines the delta to be applied when ConfigNode.Write
	// is called.
	cache map[string]interface{}
}

// NotFoundError represents the error that something is not found.
type NotFoundError struct {
	what string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s not found", e.what)
}

// Keys returns the current keys in alphabetical order.
func (c *ConfigNode) Keys() []string {
	keys := []string{}
	for key := range c.cache {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the value of key and whether it was found.
func (c *ConfigNode) Get(key string) (value interface{}, found bool) {
	value, found = c.cache[key]
	return
}

// Map returns all keys and values of the node.
func (c *ConfigNode) Map() map[string]interface{} {
	return copyCache(c.cache)
}

// Set sets key to value
func (c *ConfigNode) Set(key string, value interface{}) {
	c.cache[key] = value
}

// Update sets multiple key/value pairs.
func (c *ConfigNode) Update(kv map[string]interface{}) {
	for key, value := range kv {
		c.cache[key] = value
	}
}

// Delete removes key.
func (c *ConfigNode) Delete(key string) {
	delete(c.cache, key)
}

// copyCache copies the keys and values of one cache into a new one.
func copyCache(in map[string]interface{}) (out map[string]interface{}) {
	out = make(map[string]interface{})
	for key, value := range in {
		out[key] = value
	}
	return
}
