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

// zkMap reads the yaml data of path from zk and
// parses it into a map.
func zkMap(zk *zookeeper.Conn, path string) (map[string]interface{}, error) {
	yaml, _, err := zk.Get(path)
	if err != nil {
		return nil, err
	}
	sm := make(map[string]interface{})
	if err = goyaml.Unmarshal([]byte(yaml), sm); err != nil {
		return nil, err
	}
	return sm, nil
}

// zkMapField reads path from zk as a yaml map, and returns
// the value for field. 
func zkMapField(zk *zookeeper.Conn, path, field string) (value interface{}, err error) {
	sm, err := zkMap(zk, path)
	if err != nil {
		return "", err
	}
	value, ok := sm[field]
	if !ok {
		return "", fmt.Errorf("cannot find field %q in path %q", field, path)
	}
	return value, nil
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
