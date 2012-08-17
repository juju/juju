package mstate

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
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
	// disk holds the values in the config node before
	// any keys have been changed. It is reset on Read and Write
	// operations.
	disk map[string]interface{}
	// cache holds the current values in the config node.
	// The difference between disk and core
	// determines the delta to be applied when ConfigNode.Write
	// is called.
	core map[string]interface{}
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
	for key := range c.core {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Get returns the value of key and whether it was found.
func (c *ConfigNode) Get(key string) (value interface{}, found bool) {
	value, found = c.core[key]
	return
}

// Map returns all keys and values of the node.
func (c *ConfigNode) Map() map[string]interface{} {
	return copyMap(c.core)
}

// Set sets key to value
func (c *ConfigNode) Set(key string, value interface{}) {
	c.core[key] = value
}

// Update sets multiple key/value pairs.
func (c *ConfigNode) Update(kv map[string]interface{}) {
	for key, value := range kv {
		c.core[key] = value
	}
}

// Delete removes key.
func (c *ConfigNode) Delete(key string) {
	delete(c.core, key)
}

// copyMap copies the keys and values of one map into a new one.
func copyMap(in map[string]interface{}) (out map[string]interface{}) {
	out = make(map[string]interface{})
	for key, value := range in {
		out[key] = value
	}
	return
}

// cacheKeys returns the keys of all caches as a key=>true map.
func cacheKeys(caches ...map[string]interface{}) map[string]bool {
	keys := make(map[string]bool)
	for _, cache := range caches {
		for key := range cache {
			keys[key] = true
		}
	}
	return keys
}

// Write writes changes made to c back onto its node.  Changes are written
// as a delta applied on top of the latest version of the node, to prevent
// overwriting unrelated changes made to the node since it was last read.
func (c *ConfigNode) Write() ([]ItemChange, error) {
	changes := []ItemChange{}
	upserts := map[string]interface{}{}
	deletions := map[string]int{}
	for key := range cacheKeys(c.disk, c.core) {
		old, ondisk := c.disk[key]
		new, incore := c.core[key]
		if new == old {
			continue
		}
		var change ItemChange
		switch {
		case incore && ondisk:
			change = ItemChange{ItemModified, key, old, new}
			upserts[key] = new
		case incore && !ondisk:
			change = ItemChange{ItemAdded, key, nil, new}
			upserts[key] = new
		case ondisk && !incore:
			change = ItemChange{ItemDeleted, key, old, nil}
			deletions[key] = 1
		default:
			panic("unreachable")
		}
		changes = append(changes, change)
	}
	if len(changes) == 0 {
		return []ItemChange{}, nil
	}
	sort.Sort(itemChangeSlice(changes))
	change := bson.D{
		{"$inc", bson.D{{"version", 1}}},
		{"$set", upserts},
		{"$unset", deletions},
	}
	_, err := c.st.cfgnodes.UpsertId(c.path, change)
	if err != nil {
		return nil, fmt.Errorf("cannot write configuration node %q: %v", c.path, err)
	}
	c.disk = copyMap(c.core)
	return changes, nil
}

func newConfigNode(st *State, path string) *ConfigNode {
	return &ConfigNode{
		st:   st,
		path: path,
		core: make(map[string]interface{}),
	}
}

// cleanMap cleans the map of version and _id fields.
func cleanMap(in map[string]interface{}) {
	delete(in, "_id")
	delete(in, "version")
}

// Read (re)reads the node data into c.
func (c *ConfigNode) Read() error {
	config := map[string]interface{}{}
	err := c.st.cfgnodes.FindId(c.path).One(config)
	if err == mgo.ErrNotFound {
		c.disk = nil
		c.core = make(map[string]interface{})
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot read configuration node %q: %v", c.path, err)
	}
	cleanMap(config)
	c.disk = copyMap(config)
	c.core = copyMap(config)
	return nil
}

// readConfigNode returns the ConfigNode for path.
func readConfigNode(st *State, path string) (*ConfigNode, error) {
	c := newConfigNode(st, path)
	if err := c.Read(); err != nil {
		return nil, err
	}
	return c, nil
}

// createConfigNode writes an initial config node.
func createConfigNode(st *State, path string, values map[string]interface{}) (*ConfigNode, error) {
	c := newConfigNode(st, path)
	c.core = copyMap(values)
	_, err := c.Write()
	if err != nil {
		return nil, err
	}
	return c, nil
}
