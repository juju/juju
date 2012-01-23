// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"sort"
)

var (
	// stateChange is a common error inside the state processing.
	stateChanged = errors.New("environment state has changed")
	// zkPermAll is a convenience variable for creating new nodes.
	zkPermAll = zookeeper.WorldACL(zookeeper.PERM_ALL)
)

const (
	itemAdded = iota + 1
	itemModified
	itemDeleted
)

// ItemChange represents the change of an item in a configNode.
type ItemChange struct {
	changeType int
	key        string
	oldValue   interface{}
	newValue   interface{}
}

// String returns the item change in a readable format.
func (ic *ItemChange) String() string {
	switch ic.changeType {
	case itemAdded:
		return fmt.Sprintf("setting added: %v = %v", ic.key, ic.newValue)
	case itemModified:
		return fmt.Sprintf("setting modified: %v = %v (was %v)",
			ic.key, ic.newValue, ic.oldValue)
	case itemDeleted:
		return fmt.Sprintf("setting deleted: %v (was %v)", ic.key, ic.oldValue)
	}
	return fmt.Sprintf("unknown setting change type %d: %v = %v (was %v)",
		ic.changeType, ic.key, ic.newValue, ic.oldValue)
}

// A ConfigNode represents the data of a ZooKeeper node
// containing YAML-based settings. It manages changes to
// the data as a delta in memory, and merges them back
// onto the node when explicitly requested.
type ConfigNode struct {
	zk            *zookeeper.Conn
	path          string
	pristineCache map[string]interface{}
	cache         map[string]interface{}
}

// createConfigNode writes an initial config node.
func createConfigNode(zk *zookeeper.Conn, path string, values map[string]interface{}) (*ConfigNode, error) {
	c := &ConfigNode{
		zk:            zk,
		path:          path,
		pristineCache: copyCache(values),
		cache:         make(map[string]interface{}),
	}
	_, err := c.Write()
	if err != nil {
		return nil, err
	}
	return c, nil
}

// readConfigNode returns the ConfigNode for path.
// If required is true, an error will be returned if
// the path doesn't exist.
func readConfigNode(zk *zookeeper.Conn, path string, required bool) (*ConfigNode, error) {
	c := &ConfigNode{
		zk:            zk,
		path:          path,
		pristineCache: make(map[string]interface{}),
		cache:         make(map[string]interface{}),
	}
	yaml, _, err := c.zk.Get(c.path)
	if err != nil {
		if err == zookeeper.ZNONODE && required {
			return nil, fmt.Errorf("config %q not found", c.path)
		}
		return nil, err
	}
	if err = goyaml.Unmarshal([]byte(yaml), c.cache); err != nil {
		return nil, err
	}
	c.pristineCache = copyCache(c.cache)
	return c, nil
}

// Write writes changes made to c back onto its node.
// Changes are written as a delta applied on top of the
// latest version of the node, to prevent overwriting
// unrelated changes made to the node since it was last read.
func (c *ConfigNode) Write() ([]*ItemChange, error) {
	cache := copyCache(c.cache)
	pristineCache := copyCache(c.pristineCache)
	c.pristineCache = copyCache(cache)
	// changes is used by applyChanges to return the changes to
	// this scope.
	changes := []*ItemChange{}
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
		for key := range cacheKeys(pristineCache, cache) {
			oldValue, ok := pristineCache[key]
			if !ok {
				oldValue = missing
			}
			newValue, ok := cache[key]
			if !ok {
				newValue = missing
			}
			if oldValue != newValue {
				var change *ItemChange
				if newValue != missing {
					current[key] = newValue
					if oldValue != missing {
						change = &ItemChange{itemModified, key, oldValue, newValue}
					} else {
						change = &ItemChange{itemAdded, key, nil, newValue}
					}
				} else if _, ok := current[key]; ok {
					delete(current, key)
					change = &ItemChange{itemDeleted, key, oldValue, nil}
				}
				changes = append(changes, change)
			}
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

// Set sets key to value
func (c *ConfigNode) Set(key string, value interface{}) {
	c.cache[key] = value
}

// Delete removes key.
func (c *ConfigNode) Delete(key string) {
	delete(c.cache, key)
}

// zkRemoveTree recursively removes a tree.
func zkRemoveTree(zk *zookeeper.Conn, path string) error {
	// First recursively delete the children.
	children, _, err := zk.Children(path)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err = zkRemoveTree(zk, fmt.Sprintf("%s/%s", path, child)); err != nil {
			return err
		}
	}
	// Now delete the path itself.
	return zk.Delete(path, -1)
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
