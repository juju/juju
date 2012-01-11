// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
)

// The protocol version, which is stored in the /topology node under
// the "version" key. The protocol version should *only* be updated
// when we know that a version is in fact actually incompatible.
const topologyVersion = 1

// zkTopology is used to marshal and unmarshal the content
// of the /topology node in ZooKeeper.
type zkTopology struct {
	Version      int
	Services     map[string]*zkService
	UnitSequence map[string]int "unit-sequence"
}

// zkService represents the service data within the /topology
// node in ZooKeeper.
type zkService struct {
	Name  string
	Units map[string]*zkUnit
}

// zkUnit represents the unit data within the /topology
// node in ZooKeeper.
type zkUnit struct {
	Sequence int
	Machine  string
}

// topology is an internal helper that handles the content
// of the /topology node in ZooKeeper.
type topology struct {
	topology *zkTopology
}

// readTopology connects ZooKeeper, retrieves the data as YAML,
// parses it and returns it.
func readTopology(zk *zookeeper.Conn) (*topology, error) {
	yaml, _, err := zk.Get("/topology")
	if err != nil {
		return nil, err
	}
	return parseTopology(yaml)
}

// dump returns the topology as YAML.
func (t *topology) dump() (string, error) {
	topologyYaml, err := goyaml.Marshal(t.topology)
	if err != nil {
		return "", err
	}
	return string(topologyYaml), nil
}

// version returns the version of the topology.
func (t *topology) version() int {
	return t.topology.Version
}

// hasService returns true if a service with the given node name exists.
func (t *topology) hasService(node string) bool {
	return t.topology.Services[node] != nil
}

// serviceNodeName returns the id of a service by its name.
func (t *topology) serviceNodeName(name string) (string, error) {
	for id, svc := range t.topology.Services {
		if svc.Name == name {
			return id, nil
		}
	}
	return "", fmt.Errorf("service with name %q cannot be found", name)
}

// hasUnit returns true if a unit with given service and unit node names exists.
func (t *topology) hasUnit(serviceNode, unitNode string) bool {
	if t.hasService(serviceNode) {
		return t.topology.Services[serviceNode].Units[unitNode] != nil
	}
	return false
}

// addUnit adds a new unit and returns the sequence number. This
// sequence number will be increased monotonically for each service.
func (t *topology) addUnit(serviceNode, unitNode string) (int, error) {
	if err := t.assertService(serviceNode); err != nil {
		return -1, err
	}
	// Check if unit node name is unused.
	for node, svc := range t.topology.Services {
		if _, ok := svc.Units[unitNode]; ok {
			return -1, fmt.Errorf("unit %q already in use in servie %q", unitNode, node)
		}
	}
	// Add unit and increase sequence number.
	svc := t.topology.Services[serviceNode]
	sequenceNo := t.topology.UnitSequence[svc.Name]
	svc.Units[unitNode] = &zkUnit{Sequence: sequenceNo}
	t.topology.UnitSequence[svc.Name] += 1
	return sequenceNo, nil
}

// removeUnit removes a unit from a service.
func (t *topology) removeUnit(serviceNode, unitNode string) error {
	if err := t.assertUnit(serviceNode, unitNode); err != nil {
		return err
	}
	delete(t.topology.Services[serviceNode].Units, unitNode)
	return nil
}

// unitNodes returns the unit node names for all units of
// the service with serviceNode node name.
func (t *topology) unitNodes(serviceNode string) ([]string, error) {
	if err := t.assertService(serviceNode); err != nil {
		return nil, err
	}
	unitNodes := []string{}
	svc := t.topology.Services[serviceNode]
	for node, _ := range svc.Units {
		unitNodes = append(unitNodes, node)
	}
	return unitNodes, nil
}

// unitName returns the name of a unit by the node name of its service
// and its own node name.
func (t *topology) unitName(serviceNode, unitNode string) (string, error) {
	if err := t.assertUnit(serviceNode, unitNode); err != nil {
		return "", err
	}
	svc := t.topology.Services[serviceNode]
	unit := svc.Units[unitNode]
	return fmt.Sprintf("%s/%d", svc.Name, unit.Sequence), nil
}

// unitNodeFromSequence returns the node name of a unit by the node mame of its
// service and its sequence number.
func (t *topology) unitNodeFromSequence(serviceNode string, sequenceNo int) (string, error) {
	if err := t.assertService(serviceNode); err != nil {
		return "", err
	}
	svc := t.topology.Services[serviceNode]
	for node, unit := range svc.Units {
		if unit.Sequence == sequenceNo {
			return node, nil
		}
	}
	return "", fmt.Errorf("unit with sequence number %d cannot be found", sequenceNo)
}

// unitMachineNode returns the node name of an assigned machine of the unit. An empty
// node name means theres no machine assigned.
func (t *topology) unitMachineNode(serviceNode, unitNode string) (string, error) {
	if err := t.assertUnit(serviceNode, unitNode); err != nil {
		return "", err
	}
	unit := t.topology.Services[serviceNode].Units[unitNode]
	return unit.Machine, nil
}

// unassignUnitFromMachine unassigns the unit from its current machine.
func (t *topology) unassignUnitFromMachine(serviceNode, unitNode string) error {
	if err := t.assertUnit(serviceNode, unitNode); err != nil {
		return err
	}
	unit := t.topology.Services[serviceNode].Units[unitNode]
	if unit.Machine == "" {
		return fmt.Errorf("unit %q in service %q is not assigned to a machine", unitNode, serviceNode)
	}
	unit.Machine = ""
	return nil
}

// assertService checks if a service exists.
func (t *topology) assertService(serviceNode string) error {
	if _, ok := t.topology.Services[serviceNode]; !ok {
		return fmt.Errorf("service with id %q cannot be found", serviceNode)
	}
	return nil
}

// assertUnit checks if a service with a unit exists.
func (t *topology) assertUnit(serviceNode, unitNode string) error {
	if err := t.assertService(serviceNode); err != nil {
		return err
	}
	svc := t.topology.Services[serviceNode]
	if _, ok := svc.Units[unitNode]; !ok {
		return fmt.Errorf("unit with id %q cannot be found", serviceNode)
	}
	return nil
}

// parseTopology returns the topology represented by yaml.
func parseTopology(yaml string) (*topology, error) {
	t := &topology{topology: &zkTopology{Version: topologyVersion}}
	if err := goyaml.Unmarshal([]byte(yaml), t.topology); err != nil {
		return nil, err
	}
	if t.topology.Version != topologyVersion {
		return nil, fmt.Errorf("incompatible topology versions: got %d, want %d", t.topology.Version, topologyVersion)
	}
	return t, nil
}

// retryTopologyChange tries to change the topology with f.
// This function can read and modify the topology instance, 
// and after it returns the modified topology will be
// persisted into the /topology node. Note that this f must
// have no side-effects, since it may be called multiple times
// depending on conflict situations.
func retryTopologyChange(zk *zookeeper.Conn, f func(t *topology) error) error {
	change := func(yaml string, stat *zookeeper.Stat) (string, error) {
		var err error
		it := &topology{topology: &zkTopology{Version: 1}}
		if yaml != "" {
			if it, err = parseTopology(yaml); err != nil {
				return "", err
			}
		}
		// Apply the passed function.
		if err = f(it); err != nil {
			return "", err
		}
		return it.dump()
	}
	return zk.RetryChange("/topology", 0, zookeeper.WorldACL(zookeeper.PERM_ALL), change)
}
