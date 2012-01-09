// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
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
	Units map[string]*zkUnit
}

// zkUnit is the unit stored in ZooKeeper. It's used only internally
// for marshalling/unmarshalling.
type zkUnit struct {
	Sequence int
	Machine  string
}

// topology is an internal helper handling the topology informations
// inside of ZooKeeper. 
type topology struct {
	topology *zkTopology
}

// readTopology connects ZooKeeper, retrieves the data as YAML,
// parses it and stores it.
func readTopology(zk *zookeeper.Conn) (*topology, error) {
	t := &topology{topology: &zkTopology{Version: 1}}
	// Fetch raw topology.
	topologyYaml, _, err := zk.Get("/topology")
	if err != nil {
		return nil, err
	}
	// Parse and check it.
	if err = t.parse(topologyYaml); err != nil {
		return nil, err
	}
	return t, nil
}

// parse parses the topology delivered as YAML.
func (t topology) parse(topologyYaml string) error {
	if err := goyaml.Unmarshal([]byte(topologyYaml), t.topology); err != nil {
		return err
	}
	if t.topology.Version != Version {
		return newError("loaded topology has incompatible version '%v'", t.topology.Version)
	}
	return nil
}

// dump returns the topology as YAML.
func (t topology) dump() (string, error) {
	topologyYaml, err := goyaml.Marshal(t.topology)
	if err != nil {
		return "", err
	}
	return string(topologyYaml), nil
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
	return "", newError("service with name '%s' cannot be found", name)
}

// hasUnit returns true if a service with a given id exists.
func (t topology) hasUnit(serviceId, unitId string) bool {
	if t.hasService(serviceId) {
		return t.topology.Services[serviceId].Units[unitId] != nil
	}
	return false
}

// addUnit adds a new unit and returns the sequence number. This
// sequence number will be increased monotonically for each service.
func (t *topology) addUnit(serviceId, unitId string) (int, error) {
	if err := t.assertService(serviceId); err != nil {
		return -1, err
	}
	// Check if unit id is unused.
	for id, svc := range t.topology.Services {
		if _, ok := svc.Units[unitId]; ok {
			return -1, newError("unit id '%s' already in use in servie '%s'", unitId, id)
		}
	}
	// Add unit and increase sequence number.
	svc := t.topology.Services[serviceId]
	sequenceNo := t.topology.UnitSequence[svc.Name]
	svc.Units[unitId] = &zkUnit{Sequence: sequenceNo}
	t.topology.UnitSequence[svc.Name] += 1
	return sequenceNo, nil
}

// removeUnit removes a unit from a service.
func (t *topology) removeUnit(serviceId, unitId string) error {
	if err := t.assertUnit(serviceId, unitId); err != nil {
		return err
	}
	delete(t.topology.Services[serviceId].Units, unitId)
	return nil
}

// unitIds returns the ids of all units of a service. 
func (t topology) unitIds(serviceId string) ([]string, error) {
	if err := t.assertService(serviceId); err != nil {
		return nil, err
	}
	unitIds := []string{}
	svc := t.topology.Services[serviceId]
	for id, _ := range svc.Units {
		unitIds = append(unitIds, id)
	}
	return unitIds, nil
}

// unitName returns the name of a unit by the id of its service
// and its id. 
func (t topology) unitName(serviceId, unitId string) (string, error) {
	if err := t.assertUnit(serviceId, unitId); err != nil {
		return "", err
	}
	svc := t.topology.Services[serviceId]
	unit := svc.Units[unitId]
	return fmt.Sprintf("%s/%d", svc.Name, unit.Sequence), nil
}

// unitIdBySequence returns the id of a unit by the id of its
// service and its sequence number.
func (t topology) unitIdBySequence(serviceId string, sequenceNo int) (string, error) {
	if err := t.assertService(serviceId); err != nil {
		return "", err
	}
	svc := t.topology.Services[serviceId]
	for id, unit := range svc.Units {
		if unit.Sequence == sequenceNo {
			return id, nil
		}
	}
	return "", newError("unit with sequence number '%v' cannot be found", sequenceNo)
}

// unitMachineId returns the id of an assigned machine of the unit. And empty
// id means theres no machine assigned.
func (t *topology) unitMachineId(serviceId, unitId string) (string, error) {
	if err := t.assertUnit(serviceId, unitId); err != nil {
		return "", err
	}
	unit := t.topology.Services[serviceId].Units[unitId]
	return unit.Machine, nil
}

// unassignUnitFromMachine unassigns the unit from its current machine.
func (t *topology) unassignUnitFromMachine(serviceId, unitId string) error {
	if err := t.assertUnit(serviceId, unitId); err != nil {
		return err
	}
	unit := t.topology.Services[serviceId].Units[unitId]
	if unit.Machine == "" {
		return newError("unit '%s' in service '%s' is not assigned to a machine", unitId, serviceId)
	}
	unit.Machine = ""
	return nil
}

// assertService checks if a service exists.
func (t topology) assertService(serviceId string) error {
	if _, ok := t.topology.Services[serviceId]; !ok {
		return newError("service with id '%v' cannot be found", serviceId)
	}
	return nil
}

// assertUnit checks if a service with a unit exists.
func (t topology) assertUnit(serviceId, unitId string) error {
	if err := t.assertService(serviceId); err != nil {
		return err
	}
	svc := t.topology.Services[serviceId]
	if _, ok := svc.Units[unitId]; !ok {
		return newError("unit with id '%v' cannot be found", serviceId)
	}
	return nil
}

// retryTopologyChange tries to change the topology with a function which 
// accepts a topology instance as an argument. This function can read
// and modify the topology instance, and after it returns (or after
// the returned deferred fires) the modified topology will be
// persisted into the /topology node.  Note that this function must
// have no side-effects, since it may be called multiple times
// depending on conflict situations.

func retryTopologyChange(zk *zookeeper.Conn, f func(t *topology) error) error {
	changeContent := func(topologyYaml string, stat *zookeeper.Stat) (string, error) {
		t := &topology{topology: &zkTopology{Version: 1}}
		if topologyYaml != "" {
			t.parse(topologyYaml)
		}
		// Apply the passed function.
		if err := f(t); err != nil {
			return "", err
		}
		return t.dump()
	}
	return zk.RetryChange("/topology", 0, zookeeper.WorldACL(zookeeper.PERM_ALL), changeContent)
}
