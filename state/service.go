// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
)

// Service represents the state of a service.
type Service struct {
	topology *topology
	id       string
	name     string
}

// newService returns a new service instance
func newService(t *topology, id, name string) *Service {
	return &Service{t, id, name}
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
func (s Service) CharmId() (string, error) {
	sm, err := s.topology.getStringMap(s.zkPath())

	if err != nil {
		return "", err
	}
	if charmId, ok := sm["charm"]; ok {
		return charmId, nil
	}
	return "", ErrServiceHasNoCharmId
}

// zkPath returns the path within ZooKeeper.
func (s Service) zkPath() string {
	return fmt.Sprintf("/services/%s", s.id)
}

// zkConfigPath returns the ZooKeeper path to the configuration.
func (s Service) zkConfigPath() string {
	return fmt.Sprintf("%s/config", s.zkPath())
}

// zkExposedPath, if exists in ZooKeeper, indicates, that a
// service is exposed.
func (s Service) zkExposedPath() string {
	return fmt.Sprintf("/services/%s/exposed", s.id)
}
