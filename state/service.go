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
	id   string
	name string
}

// Id returns the service id.
func (s Service) Id() string {
	return s.id
}

// Name returns the service name.
func (s Service) Name() string {
	return s.name
}

// CharmId returns the charm id this service is supposed
// to use.
func (s Service) CharmId() (charmId string, err error) {
	return zkStringMapField(s.zk, s.zkPath(), "charm")
}

// AddUnit() adds a new unit.
func (s Service) AddUnit() (*Unit, error) {
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
	// Define the add function for topology and call it.
	unitId := strings.Split(path, "/")[2]
	sequenceNo := -1
	addUnit := func(t *topology) error {
		if !t.hasService(s.id) {
			return newError("service state has changed")
		}
		sequenceNo, err = t.addUnit(s.id, unitId)
		if err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, addUnit); err != nil {
		return nil, err
	}
	return &Unit{s.zk, unitId, s.id, s.name, sequenceNo}, nil
}

// RemoveUnit() removes a unit.
func (s Service) RemoveUnit(unit *Unit) error {
	// First unassign from machine if currently assigned.
	unit.UnassignFromMachine()
	// Define the remove function fir topology and call it.
	removeUnit := func(t *topology) error {
		if !t.hasService(s.id) || !t.hasUnit(s.id, unit.id) {
			return newError("service state has changed")
		}
		if err := t.removeUnit(s.id, unit.id); err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, removeUnit); err != nil {
		return err
	}
	return zkRemoveTree(s.zk, fmt.Sprintf("/units/%s", unit.id))
}

// Unit returns a unit by name.
func (s Service) Unit(unitName string) (*Unit, error) {
	serviceName, sequenceNo, err := parseUnitName(unitName)
	if err != nil {
		return nil, err
	}
	// Check for matching service name.
	if serviceName != s.name {
		return nil, newError("service name '%v' of unit does not match with service name '%v'",
			serviceName, s.name)
	}
	// Check if the topology has been changed.
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.id) {
		return nil, newError("service state has changed")
	}
	// Read unit id and create unit.
	unitId, err := topology.unitIdBySequence(s.id, sequenceNo)
	if err != nil {
		return nil, err
	}
	return &Unit{s.zk, unitId, s.id, s.name, sequenceNo}, nil
}

// AllUnits returns all units of the service.
func (s Service) AllUnits() ([]*Unit, error) {
	// Check if the topology has changed.
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.id) {
		return nil, newError("service state has changed")
	}
	// Retrieve unit ids.
	unitIds, err := topology.unitIds(s.id)
	if err != nil {
		return nil, err
	}
	// Assemble units.
	units := []*Unit{}
	for _, unitId := range unitIds {
		unitName, err := topology.unitName(s.id, unitId)
		if err != nil {
			return nil, err
		}
		serviceName, sequenceNo, err := parseUnitName(unitName)
		if err != nil {
			return nil, err
		}
		units = append(units, &Unit{s.zk, unitId, s.id, serviceName, sequenceNo})
	}
	return units, nil
}

// UnitNames returns the names of the services units.
func (s Service) UnitNames() ([]string, error) {
	// Check if the topology has changed.
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.hasService(s.id) {
		return nil, newError("service state has changed")
	}
	// Retrieve unit ids.
	unitIds, err := topology.unitIds(s.id)
	if err != nil {
		return nil, err
	}
	// Assemble unit names.
	unitNames := []string{}
	for _, unitId := range unitIds {
		unitName, err := topology.unitName(s.id, unitId)
		if err != nil {
			return nil, err
		}
		unitNames = append(unitNames, unitName)
	}
	return unitNames, nil
}

// zkPath returns the ZooKeeper base path for the service.
func (s Service) zkPath() string {
	return fmt.Sprintf("/services/%s", s.id)
}

// zkConfigPath returns the ZooKeeper path for the service configuration.
func (s Service) zkConfigPath() string {
	return fmt.Sprintf("%s/config", s.zkPath())
}

// zkExposedPath, if exists in ZooKeeper, indicates, that a
// service is exposed.
func (s Service) zkExposedPath() string {
	return fmt.Sprintf("/services/%s/exposed", s.id)
}
