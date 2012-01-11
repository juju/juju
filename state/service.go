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
	key  string
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
	return value.(string), nil
}

// AddUnit() adds a new unit.
func (s *Service) AddUnit() (*Unit, error) {
	// Get charm id and create ZooKeeper node.
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
	key := strings.Split(path, "/")[2]
	sequenceNo := -1
	addUnit := func(t *topology) error {
		if !t.hasService(s.key) {
			return stateChanged
		}
		sequenceNo, err = t.addUnit(s.key, key)
		if err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, addUnit); err != nil {
		return nil, err
	}
	return &Unit{s.zk, key, s.key, s.name, sequenceNo}, nil
}

// RemoveUnit() removes a unit.
func (s *Service) RemoveUnit(unit *Unit) error {
	// First unassign from machine if currently assigned.
	if err := unit.UnassignFromMachine(); err != nil {
		return err
	}
	removeUnit := func(t *topology) error {
		if !t.hasService(s.key) || !t.hasUnit(s.key, unit.key) {
			return stateChanged
		}
		if err := t.removeUnit(s.key, unit.key); err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, removeUnit); err != nil {
		return err
	}
	return zkRemoveTree(s.zk, fmt.Sprintf("/units/%s", unit.key))
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
	if !topology.hasService(s.key) {
		return nil, stateChanged
	}
	// Read unit key and create unit.
	key, err := topology.unitKeyFromSequence(s.key, sequenceNo)
	if err != nil {
		return nil, err
	}
	return &Unit{s.zk, key, s.key, s.name, sequenceNo}, nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() ([]*Unit, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.key) {
		return nil, stateChanged
	}
	keys, err := topology.unitKeys(s.key)
	if err != nil {
		return nil, err
	}
	// Assemble units.
	units := []*Unit{}
	for _, key := range keys {
		unitName, err := topology.unitName(s.key, key)
		if err != nil {
			return nil, err
		}
		serviceName, sequenceNo, err := parseUnitName(unitName)
		if err != nil {
			return nil, err
		}
		units = append(units, &Unit{s.zk, key, s.key, serviceName, sequenceNo})
	}
	return units, nil
}

// UnitNames returns the names of all units of s.
func (s *Service) UnitNames() ([]string, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.key) {
		return nil, stateChanged
	}
	keys, err := topology.unitKeys(s.key)
	if err != nil {
		return nil, err
	}
	// Assemble unit names.
	unitNames := []string{}
	for _, key := range keys {
		unitName, err := topology.unitName(s.key, key)
		if err != nil {
			return nil, err
		}
		unitNames = append(unitNames, unitName)
	}
	return unitNames, nil
}

// zkKey returns ZooKeeper key of the service.
func (s *Service) zkKey() string {
	return s.key
}

// zkPath returns the ZooKeeper base path for the service.
func (s *Service) zkPath() string {
	return fmt.Sprintf("/services/%s", s.key)
}

// zkConfigPath returns the ZooKeeper path for the service configuration.
func (s *Service) zkConfigPath() string {
	return fmt.Sprintf("/services/%s/config", s.key)
}

// zkExposedPath, if exists in ZooKeeper, indicates, that a
// service is exposed.
func (s *Service) zkExposedPath() string {
	return fmt.Sprintf("/services/%s/exposed", s.key)
}
