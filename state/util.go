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

// newError allows a quick error creation with arguments.
func newError(text string, args ...interface{}) error {
	return errors.New(fmt.Sprintf("state: "+text, args...))
}

// zkStringMap returns a map of strings to strings read from ZK
// based on a path.
func zkStringMap(zk *zookeeper.Conn, path string) (map[string]string, error) {
	// Fetch raw data.
	raw, _, err := zk.Get(path)
	if err != nil {
		return nil, err
	}
	// Unmarshal it.
	sm := make(map[string]string)
	if err = goyaml.Unmarshal([]byte(raw), sm); err != nil {
		return nil, err
	}
	return sm, nil
}

// zkStringMapField returns a field our of a string map returned by stringMap().
func zkStringMapField(zk *zookeeper.Conn, path, field string) (string, error) {
	// Get the map.
	sm, err := zkStringMap(zk, path)
	if err != nil {
		return "", err
	}
	// Look if field exists.
	value, ok := sm[field]
	if !ok {
		return "", newError("cannot find field '%s' in path '%s'", field, path)
	}
	return value, nil
}

// zkRemoveTree recursively removes a tree.
func zkRemoveTree(zk *zookeeper.Conn, path string) error {
	// First recursively delete the cildren.
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
