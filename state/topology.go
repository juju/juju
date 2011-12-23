// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
)

const (
	Version = 1
)

// ZkTopology is the data stored in ZK below "/topology". It's
// used only internally for marshalling/unmarshalling. All
// ZkXyz types sadly have to be public for this.
type ZkTopology struct {
	Version      int
	Services     map[string]*ZkService
	UnitSequence map[string]int "unit-sequence"
}

// newZkTopology creates an initial empty topology with
// version 1.
func newZkTopology() *ZkTopology {
	return &ZkTopology{
		Version:      1,
		Services:     make(map[string]*ZkService),
		UnitSequence: make(map[string]int),
	}
}

// ZkService is the service stored in ZK. It's used only 
// internally for marshalling/unmarshalling.
type ZkService struct {
	Name    string
	Charm   string
	Units   map[string]*ZkUnit
	Exposed bool
}

// ZkUnit is the unit stored in ZK. It's used only internally
// for marshalling/unmarshalling.
type ZkUnit struct {
	Sequence int
	Exposed  bool
}

// topology is an internal helper handling the topology informations
// inside of ZooKeeper. It also keeps the connection to ZooKeeper for
// easier usage by the values. The raw topology is kept to check for
// changes before unmarshalling.
type topology struct {
	zk                   *zookeeper.Conn
	currentRawZkTopology string
	currentZkTopology    *ZkTopology
}

// newTopology connects ZooKeeper, retrieves the data as YAML,
// parses it and stores it.
func newTopology(zk *zookeeper.Conn) (*topology, error) {
	t := &topology{
		zk:                zk,
		currentZkTopology: newZkTopology(),
	}
	if err := t.reload(); err != nil {
		return nil, err
	}
	return t, nil
}

// get provides a simple access to ZooKeeper including unmarshalling
// into a map of strings to strings.
func (t *topology) getStringMap(path string) (map[string]string, error) {
	// Fetch raw data.
	raw, _, err := t.zk.Get(path)
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

// reload reloads the topology. Parsing is only done again if the
// raw string has changed.
func (t *topology) reload() error {
	// Fetch raw topology.
	raw, _, err := t.zk.Get("/topology")
	if err != nil {
		return err
	}
	// Check and unmarshal it.
	if raw != t.currentRawZkTopology {
		t.currentRawZkTopology = raw
		if err = goyaml.Unmarshal([]byte(raw), t.currentZkTopology); err != nil {
			return err
		}
		if t.currentZkTopology.Version != Version {
			return ErrIncompatibleVersion
		}
	}
	return nil
}

// version returns the version of the topology.
func (t *topology) version() int {
	return t.currentZkTopology.Version
}

// serviceIdByName returns the id of a service by its name.
func (t *topology) serviceIdByName(n string) (string, error) {
	if err := t.reload(); err != nil {
		return "", err
	}
	for id, svc := range t.currentZkTopology.Services {
		if svc.Name == n {
			return id, nil
		}
	}
	return "", ErrServiceNotFound
}
