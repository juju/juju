// launchpad.net/juju/state
//
// Copyright (c) 2011 Canonical Ltd.

package state

import (
	"fmt"
	"sync"
)

// Service represents the state of a service
// and its subordinate parts.
type Service struct {
	writeLock sync.Mutex
	topology  *topology
	id        string
	Exposed   bool             "exposed"
	Name      string           "name"
	CharmId   string           "charm"
	Units     map[string]*Unit "units"
}

// Id returns the service id.
func (s Service) Id() string {
	return s.id
}

// Unit returns the unit with the given id.
func (s Service) Unit(id string) (*Unit, error) {
	unit, ok := s.Units[id]

	if ok {
		unit.topology = s.topology
		unit.id = id

		return unit, nil
	}

	return nil, ErrUnitNotFound
}

// sync synchronizes the service after an update event. This
// is done recurively with all entities below.
func (s *Service) sync(newSvc *Service) error {
	s.writeLock.Lock()
	defer s.writeLock.Unlock()

	// 1. Own fields.
	s.Exposed = newSvc.Exposed
	s.Name = newSvc.Name
	s.CharmId = newSvc.CharmId

	// 2. Units. First store current ids, then update
	// or add new units and at last remove invalid
	// units.
	currentUnitIds := make(map[string]bool)

	for id, _ := range s.Units {
		currentUnitIds[id] = true
	}

	for id, u := range newSvc.Units {
		if currentUnit, ok := s.Units[id]; ok {
			// Sync unit and mark as synced.
			// OPEN: How to handle exposed units?
			if err := currentUnit.sync(u); err != nil {
				return err
			}

			delete(currentUnitIds, id)
		} else {
			// Add new unit.
			s.Units[id] = u
		}
	}

	for id, _ := range currentUnitIds {
		// Mark exposed, for those who still have references.
		s.Units[id].Exposed = true

		delete(s.Units, id)
	}

	return nil
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
