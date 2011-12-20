// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

// --------------------
// IMPORT
// --------------------

import (
	"fmt"
)

// --------------------
// SERVICE
// --------------------

// Service represents the state of one service and
// provides also the access to the subordinate parts.
type Service struct {
	topology *topology
	id       string
	name     string
}

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

// CharmId returns the charm id this service is supposed to use.
func (s Service) CharmId() string {
	cid, err := s.topology.getString(s.zookeeperPath() + "/charm")

	if err != nil {
		// TODO: Ouch! Error handling.
		panic("TODO: Error handling!")
	}

	return cid
}

// zookeeperPath returns the path within ZooKeeper.
func (s Service) zookeeperPath() string {
	return fmt.Sprintf("/services/%s", s.id)
}

// configPath returns the ZooKeeper path to the configuration.
func (s Service) configPath() string {
	return fmt.Sprintf("%s/config", s.zookeeperPath())
}

// exposedPath, if exists in ZooKeeper, indicates, that a
// service is exposed.
func (s Service) exposedPath() string {
	return fmt.Sprintf("/services/%s/exposed", s.id)
}

// EOF
