// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	pathpkg "path"
	"sort"
)

var (
	// stateChange is a common error inside the state processing.
	stateChanged = errors.New("environment state has changed")
	// zkPermAll is a convenience variable for creating new nodes.
	zkPermAll = zookeeper.WorldACL(zookeeper.PERM_ALL)
)

const (
	ItemAdded = iota + 1
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
	zk   *zookeeper.Conn
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

func newConfigNode(zk *zookeeper.Conn, path string) *ConfigNode {
	return &ConfigNode{
		zk:    zk,
		path:  path,
		cache: make(map[string]interface{}),
	}
}

// createConfigNode writes an initial config node.
func createConfigNode(zk *zookeeper.Conn, path string, values map[string]interface{}) (*ConfigNode, error) {
	c := newConfigNode(zk, path)
	c.cache = copyCache(values)
	_, err := c.Write()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// parseConfigNode creates a config node based on a pre-read content.
func parseConfigNode(zk *zookeeper.Conn, path, content string) (*ConfigNode, error) {
	c := newConfigNode(zk, path)
	if err := c.setPristineContent(content); err != nil {
		return nil, err
	}
	c.cache = copyCache(c.pristineCache)
	return c, nil
}

// setPristineContent sets the currently known contents of the node.
// It does not affect the user-set contents.
func (c *ConfigNode) setPristineContent(content string) error {
	if err := goyaml.Unmarshal([]byte(content), &c.pristineCache); err != nil {
		return err
	}
	return nil
}

// readConfigNode returns the ConfigNode for path.
func readConfigNode(zk *zookeeper.Conn, path string) (*ConfigNode, error) {
	c := newConfigNode(zk, path)
	if err := c.Read(); err != nil {
		return nil, err
	}
	return c, nil
}

// Read (re)reads the node data into c.
func (c *ConfigNode) Read() error {
	yaml, _, err := c.zk.Get(c.path)
	if err != nil {
		if !zookeeper.IsError(err, zookeeper.ZNONODE) {
			return err
		}
	}
	if err := c.setPristineContent(yaml); err != nil {
		return err
	}
	c.cache = copyCache(c.pristineCache)
	return nil
}

// Write writes changes made to c back onto its node.
// Changes are written as a delta applied on top of the
// latest version of the node, to prevent overwriting
// unrelated changes made to the node since it was last read.
func (c *ConfigNode) Write() ([]ItemChange, error) {
	// changes is used by applyChanges to return the changes to
	// this scope.
	changes := []ItemChange{}
	// nil is a possible value for a key, so we use missing as
	// a marker to simplify the algorithm below.
	missing := new(bool)
	applyChanges := func(yaml string, stat *zookeeper.Stat) (string, error) {
		changes = changes[:0]
		current := make(map[string]interface{})
		if yaml != "" {
			if err := goyaml.Unmarshal([]byte(yaml), current); err != nil {
				return "", err
			}
		}
		for key := range cacheKeys(c.pristineCache, c.cache) {
			oldValue, ok := c.pristineCache[key]
			if !ok {
				oldValue = missing
			}
			newValue, ok := c.cache[key]
			if !ok {
				newValue = missing
			}
			if oldValue == newValue {
				continue
			}
			var change ItemChange
			if newValue != missing {
				current[key] = newValue
				if oldValue != missing {
					change = ItemChange{ItemModified, key, oldValue, newValue}
				} else {
					change = ItemChange{ItemAdded, key, nil, newValue}
				}
			} else if _, ok := current[key]; ok {
				delete(current, key)
				change = ItemChange{ItemDeleted, key, oldValue, nil}
			}
			changes = append(changes, change)
		}
		currentYaml, err := goyaml.Marshal(current)
		if err != nil {
			return "", err
		}
		return string(currentYaml), nil
	}
	if err := c.zk.RetryChange(c.path, 0, zkPermAll, applyChanges); err != nil {
		return nil, err
	}
	sort.Sort(itemChangeSlice(changes))
	c.pristineCache = copyCache(c.cache)
	return changes, nil
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

// zkRemoveTree recursively removes a zookeeper node and all its
// children.  It does not delete "/zookeeper" or the root node itself
// and it does not consider deleting a nonexistent node to be an error.
func zkRemoveTree(zk *zookeeper.Conn, path string) (err error) {
	defer errorContext(&err, "can't clean up data: %v")
	// If we try to delete the zookeeper node (for example when
	// calling ZkRemoveTree(zk, "/")) we silently ignore it.
	if path == "/zookeeper" {
		return
	}
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			return nil
		}
		return
	}
	for _, child := range children {
		if err = zkRemoveTree(zk, pathpkg.Join(path, child)); err != nil {
			return
		}
	}
	// Now delete the path itself unless it's the root node.
	if path == "/" {
		return nil
	}
	err = zk.Delete(path, -1)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		return err
	}
	return nil
}

// copyCache copies the keys and values of one cache into a new one.
func copyCache(in map[string]interface{}) (out map[string]interface{}) {
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

// errorContext enriches the passed error with the format string and
// its arguments. The error itself is the last argument.
func errorContext(err *error, format string, args ...interface{}) {
	if *err != nil {
		*err = fmt.Errorf(format, append(args, *err)...)
	}
}
