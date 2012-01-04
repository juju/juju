// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
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
	return &Unit{s.zk, unitId, s.id, serviceName, sequenceNo}, nil
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
