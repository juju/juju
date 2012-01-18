// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
)

var (
	stateChanged = errors.New("environment state has changed")
)

const (
	itemAdded = iota
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

// ConfigNode provides a map like interface around a ZooKeeper node
// containing serialised YAML data. The map provided represents the
// local view of all node data. write() writes this information into 
// the ZooKeeper node, using a retry until success and merges against 
// any existing keys in ZooKeeper. The state of this object always 
// represents the product of the pristine settings (from ZooKeeper) 
// and the pending writes.
type ConfigNode struct {
	zk            *zookeeper.Conn
	path          string
	pristineCache map[string]interface{}
	cache         map[string]interface{}
}

// createConfigNode writes an initial config node, e.g. when adding
// a new service.
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

// readConfigNode reads the current ZooKeeper config data for a node.
func readConfigNode(zk *zookeeper.Conn, path string, required bool) (*ConfigNode, error) {
	c := &ConfigNode{
		zk:            zk,
		path:          path,
		pristineCache: make(map[string]interface{}),
		cache:         make(map[string]interface{}),
	}
	yaml, _, err := c.zk.Get(c.path)
	if err != nil {
		if zkErr, ok := err.(zookeeper.Error); ok {
			// -101 is the error code for not existing nodes.
			if zkErr == -101 && required {
				return nil, fmt.Errorf("config %q not found", c.path)
			}
		}
		return nil, err
	}
	if err = goyaml.Unmarshal([]byte(yaml), c.cache); err != nil {
		return nil, err
	}
	c.pristineCache = copyCache(c.cache)
	return c, nil
}

// write the current config data back to ZooKeper, the data will be
// merged, the write buffer resetted.
func (c *ConfigNode) Write() ([]*ItemChange, error) {
	cache := copyCache(c.cache)
	pristineCache := copyCache(c.pristineCache)
	c.pristineCache = copyCache(cache)
	// Changes records the changes done in applyChanges.
	missing := new(bool)
	changes := []*ItemChange{}
	logChange := func(changeType int, key string, oldValue, newValue interface{}) {
		changes = append(changes, &ItemChange{changeType, key, oldValue, newValue})
	}
	applyChanges := func(yaml string, stat *zookeeper.Stat) (string, error) {
		changes = []*ItemChange{}
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
				if newValue != missing {
					current[key] = newValue
					if oldValue != missing {
						logChange(itemModified, key, oldValue, newValue)
					} else {
						logChange(itemAdded, key, nil, newValue)
					}
				} else if _, ok := current[key]; ok {
					delete(current, key)
					logChange(itemDeleted, key, oldValue, nil)
				}
			}
		}
		currentYaml, err := goyaml.Marshal(current)
		if err != nil {
			return "", err
		}
		return string(currentYaml), nil
	}
	if err := c.zk.RetryChange(c.path, 0, zookeeper.WorldACL(zookeeper.PERM_ALL), applyChanges); err != nil {
		return nil, err
	}
	return changes, nil
}

// Keys returns the keys of a config node as slice.
func (c *ConfigNode) Keys() []string {
	keys := []string{}
	for key := range c.cache {
		keys = append(keys, key)
	}
	return keys
}

// Get retrieves a value by its key and returns nil if it doesn't exist.
func (c *ConfigNode) Get(key string) interface{} {
	return c.cache[key]
}

// GetDefault retrieves a value by its key and returns the default if
// it doesn't exist.
func (c *ConfigNode) GetDefault(key string, dflt interface{}) interface{} {
	if value, ok := c.cache[key]; ok {
		return value
	}
	return dflt
}

// Set sets the value for a given key. If the key exist it
// returns the old value.
func (c *ConfigNode) Set(key string, newValue interface{}) interface{} {
	oldValue := c.cache[key]
	c.cache[key] = newValue
	return oldValue
}

// Delete removes a given key and value from the cache. If the key exist it
// return the value.
func (c *ConfigNode) Delete(key string) interface{} {
	oldValue := c.cache[key]
	delete(c.cache, key)
	return oldValue
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

// cacheKeys creates a slice of the keys of multiple caches.
func cacheKeys(caches ...map[string]interface{}) map[string]bool {
	keys := make(map[string]bool)
	for _, cache := range caches {
		for key := range cache {
			keys[key] = true
		}
	}
	return keys
}
