// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"strings"
)

// Service represents the state of a service.
type Service struct {
	zk   *zookeeper.Conn
	node string
	name string
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.name
}

// CharmId returns the charm id this service is supposed
// to use.
func (s *Service) CharmId() (charmId string, err error) {
	value, err := zkMapField(s.zk, s.zkPath(), "charm")
	if err != nil {
		return "", nil
	}
	if s, ok := value.(string); ok {
		return s, nil
	}
	return "", fmt.Errorf("charm has illegal type")
}

// AddUnit() adds a new unit.
func (s *Service) AddUnit() (*Unit, error) {
	// Get charm id and create node.
	charmId, err := s.CharmId()
	if err != nil {
		return nil, err
	}
	unitData := map[string]string{"charm": charmId}
	unitYaml, err := goyaml.Marshal(unitData)
	if err != nil {
		return nil, err
	}
	path, err := s.zk.Create("/units/unit-", string(unitYaml), zookeeper.SEQUENCE, zookeeper.WorldACL(zookeeper.PERM_ALL))
	if err != nil {
		return nil, err
	}
	nodeName := strings.Split(path, "/")[2]
	sequenceNo := -1
	addUnit := func(t *topology) error {
		if !t.hasService(s.node) {
			return stateChanged
		}
		sequenceNo, err = t.addUnit(s.node, nodeName)
		if err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, addUnit); err != nil {
		return nil, err
	}
	return &Unit{s.zk, nodeName, s.node, s.name, sequenceNo}, nil
}

// RemoveUnit() removes a unit.
func (s *Service) RemoveUnit(unit *Unit) error {
	// First unassign from machine if currently assigned.
	if err := unit.UnassignFromMachine(); err != nil {
		return err
	}
	removeUnit := func(t *topology) error {
		if !t.hasService(s.node) || !t.hasUnit(s.node, unit.node) {
			return stateChanged
		}
		if err := t.removeUnit(s.node, unit.node); err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, removeUnit); err != nil {
		return err
	}
	return zkRemoveTree(s.zk, fmt.Sprintf("/units/%s", unit.node))
}

// Unit returns the service's unit with name.
func (s *Service) Unit(name string) (*Unit, error) {
	serviceName, sequenceNo, err := parseUnitName(name)
	if err != nil {
		return nil, err
	}
	// Check for matching service name.
	if serviceName != s.name {
		return nil, fmt.Errorf("can't find unit %q on service %q", name, s.name)
	}
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.node) {
		return nil, stateChanged
	}
	// Read unit node name and create unit.
	nodeName, err := topology.unitNodeFromSequence(s.node, sequenceNo)
	if err != nil {
		return nil, err
	}
	return &Unit{s.zk, nodeName, s.node, s.name, sequenceNo}, nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() ([]*Unit, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.node) {
		return nil, stateChanged
	}
	nodeNames, err := topology.unitNodes(s.node)
	if err != nil {
		return nil, err
	}
	// Assemble units.
	units := []*Unit{}
	for _, nodeName := range nodeNames {
		unitName, err := topology.unitName(s.node, nodeName)
		if err != nil {
			return nil, err
		}
		serviceName, sequenceNo, err := parseUnitName(unitName)
		if err != nil {
			return nil, err
		}
		units = append(units, &Unit{s.zk, nodeName, s.node, serviceName, sequenceNo})
	}
	return units, nil
}

// UnitNames returns the names of all units of service.
func (s *Service) UnitNames() ([]string, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.node) {
		return nil, stateChanged
	}
	nodeNames, err := topology.unitNodes(s.node)
	if err != nil {
		return nil, err
	}
	// Assemble unit names.
	unitNames := []string{}
	for _, nodeName := range nodeNames {
		unitName, err := topology.unitName(s.node, nodeName)
		if err != nil {
			return nil, err
		}
		unitNames = append(unitNames, unitName)
	}
	return unitNames, nil
}

// zkNodeName returns ZooKeeper node name of the service.
func (s *Service) zkNodeName() string {
	return s.node
}

// zkPath returns the ZooKeeper base path for the service.
func (s *Service) zkPath() string {
	return fmt.Sprintf("/services/%s", s.node)
}

// zkConfigPath returns the ZooKeeper path for the service configuration.
func (s *Service) zkConfigPath() string {
	return fmt.Sprintf("/services/%s/config", s.node)
}

// zkExposedPath, if exists in ZooKeeper, indicates, that a
// service is exposed.
func (s *Service) zkExposedPath() string {
	return fmt.Sprintf("/services/%s/exposed", s.node)
}
