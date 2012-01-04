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

// ZkTopology is the data stored in ZooKeeper inside "/topology". It's
// used only internally for marshalling/unmarshalling. All
// ZkXyz types sadly have to be public for this.
type zkTopology struct {
	Version      int
	Services     map[string]*zkService
	UnitSequence map[string]int "unit-sequence"
}

// zkService is the service stored in ZooKeeper. It's used only 
// internally for marshalling/unmarshalling.
type zkService struct {
	Name  string
	Charm string
	Units map[string]*zkUnit
}

// zkUnit is the unit stored in ZooKeeper. It's used only internally
// for marshalling/unmarshalling.
type zkUnit struct {
	Sequence int
}

// topology is an internal helper handling the topology informations
// inside of ZooKeeper. 
type topology struct {
	raw      string
	topology *zkTopology
}

// readTopology connects ZooKeeper, retrieves the data as YAML,
// parses it and stores it.
func readTopology(zk *zookeeper.Conn) (t *topology, err error) {
	t = &topology{topology: &zkTopology{Version: 1}}
	// Fetch raw topology.
	t.raw, _, err = zk.Get("/topology")
	if err != nil {
		return nil, err
	}
	// Unmarshal and check it.
	if err = goyaml.Unmarshal([]byte(t.raw), t.topology); err != nil {
		return nil, err
	}
	if t.topology.Version != Version {
		return nil, newError("loaded topology has incompatible version '%v'", t.topology.Version)
	}
	return t, nil
}

// version returns the version of the topology.
func (t topology) version() int {
	return t.topology.Version
}

// hasService returns true if a service with the given id exists.
func (t topology) hasService(serviceId string) bool {
	return t.topology.Services[serviceId] != nil
}

// serviceIdByName returns the id of a service by its name.
func (t topology) serviceIdByName(name string) (string, error) {
	for id, svc := range t.topology.Services {
		if svc.Name == name {
			return id, nil
		}
	}
	return "", newError("service with name '%v' cannot be found", name)
}

// unitIdBySequence returns the id of a unit by the id of its
// service and its sequence number.
func (t topology) unitIdBySequence(serviceId string, sequenceNo int) (string, error) {
	if svc, ok := t.topology.Services[serviceId]; ok {
		for id, unit := range svc.Units {
			if unit.Sequence == sequenceNo {
				return id, nil
			}
		}
		return "", newError("unit with sequence number '%v' cannot be found", sequenceNo)
	}
	return "", newError("service with id '%v' cannot be found", serviceId)
}
