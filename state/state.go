// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

// The state package enables reading, observing, and changing
// the state stored in ZooKeeper of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
	"strings"
)

// State represents the state of an environment
// managed by juju.
type State struct {
	zk *zookeeper.Conn
}

// AddMachine creates a new machine state.
func (s *State) AddMachine() (*Machine, error) {
	path, err := s.zk.Create("/machines/machine-", "", zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, err
	}
	key := strings.Split(path, "/")[2]
	addMachine := func(t *topology) error {
		return t.AddMachine(key)
	}
	if err = retryTopologyChange(s.zk, addMachine); err != nil {
		return nil, err
	}
	return &Machine{s.zk, key}, nil
}

// RemoveMachine removes the machine with the given id.
func (s *State) RemoveMachine(id int) (bool, error) {
	key := fmt.Sprintf("machine-%010d", id)
	mustDelete := false
	removeMachine := func(t *topology) error {
		// Removing a non-existing machine again won't fail, since
		// the end intention is preserved. This makes dealing
		// with concurrency easier.
		if t.HasMachine(key) {
			hasUnits, err := t.MachineHasUnits(key)
			if err != nil {
				return err
			}
			if hasUnits {
				return fmt.Errorf("machine %d in use", id)
			}
			t.RemoveMachine(key)
			mustDelete = true
		} else {
			mustDelete = false
		}
		return nil
	}
	if err := retryTopologyChange(s.zk, removeMachine); err != nil {
		return false, err
	}
	if mustDelete {
		// The node has to be deleted. If this is interrupted
		// here, the node will stay around. This is not a big
		// deal since it's not being referenced by the topology
		// anymore.
		zkRemoveTree(s.zk, fmt.Sprintf("/machines/%s", key))
	}
	return mustDelete, nil
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	key := fmt.Sprintf("machine-%010d", id)
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.HasMachine(key) {
		return nil, fmt.Errorf("machine %d not found", id)
	}
	return &Machine{s.zk, key}, nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() ([]*Machine, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	machines := []*Machine{}
	for _, key := range topology.MachineKeys() {
		machines = append(machines, &Machine{s.zk, key})
	}
	return machines, nil
}

// AddCharm creates a new charm state based on a charm, its URL
// and its bundle URL.
func (s *State) AddCharm(ch charm.Charm, curl *charm.URL, url string) (*Charm, error) {
	data := &charmData{
		Meta:      ch.Meta(),
		Config:    ch.Config(),
		BundleURL: url,
	}
	yaml, err := goyaml.Marshal(data)
	if err != nil {
		return nil, err
	}
	path, err := charmPath(curl)
	if err != nil {
		return nil, err
	}
	_, err = s.zk.Create(path, string(yaml), 0, zkPermAll)
	if err != nil {
		return nil, err
	}
	return newCharm(s, curl, data), nil
}

// Charm returns a charm by the given id.
func (s *State) Charm(curl *charm.URL) (*Charm, error) {
	path, err := charmPath(curl)
	if err != nil {
		return nil, err
	}
	yaml, _, err := s.zk.Get(path)
	if err == zookeeper.ZNONODE {
		return nil, fmt.Errorf("charm not found: %q", curl)
	}
	if err != nil {
		return nil, err
	}
	data := &charmData{}
	if err := goyaml.Unmarshal([]byte(yaml), data); err != nil {
		return nil, err
	}
	return newCharm(s, curl, data), nil
}

// AddService creates a new service with the given unique name
// and the charm state.
func (s *State) AddService(name string, ch *Charm) (*Service, error) {
	details := map[string]interface{}{"charm": ch.URL().String()}
	yaml, err := goyaml.Marshal(details)
	if err != nil {
		return nil, err
	}
	path, err := s.zk.Create("/services/service-", string(yaml), zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, err
	}
	key := strings.Split(path, "/")[2]
	service := &Service{s, key, name}
	// Create an empty configuration node.
	_, err = createConfigNode(s.zk, service.zkConfigPath(), map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	addService := func(t *topology) error {
		if _, err := t.ServiceKey(name); err == nil {
			// No error, so service name already in use.
			return fmt.Errorf("service name %q is already in use", name)
		}
		return t.AddService(key, name)
	}
	if err = retryTopologyChange(s.zk, addService); err != nil {
		return nil, err
	}
	return service, nil
}

// RemoveService removes a service from the state. It will
// also remove all its units and break any of its existing
// relations.
func (s *State) RemoveService(svc *Service) error {
	// TODO Remove relations first, to prevent spurious hook execution.

	// Remove the units.
	units, err := svc.AllUnits()
	if err != nil {
		return err
	}
	for _, unit := range units {
		if err = svc.RemoveUnit(unit); err != nil {
			return err
		}
	}
	// Remove the service from the topology.
	removeService := func(t *topology) error {
		if !t.HasService(svc.key) {
			return stateChanged
		}
		t.RemoveService(svc.key)
		return nil
	}
	if err = retryTopologyChange(s.zk, removeService); err != nil {
		return err
	}
	return zkRemoveTree(s.zk, svc.zkPath())
}

// Service returns a service state by name.
func (s *State) Service(name string) (*Service, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	key, err := topology.ServiceKey(name)
	if err != nil {
		return nil, err
	}
	return &Service{s, key, name}, nil
}

// AllServices returns all deployed services in the environment.
func (s *State) AllServices() ([]*Service, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	services := []*Service{}
	for _, key := range topology.ServiceKeys() {
		name, err := topology.ServiceName(key)
		if err != nil {
			return nil, err
		}
		services = append(services, &Service{s, key, name})
	}
	return services, nil
}

// Unit returns a unit by name.
func (s *State) Unit(name string) (*Unit, error) {
	serviceName, _, err := parseUnitName(name)
	if err != nil {
		return nil, err
	}
	service, err := s.Service(serviceName)
	if err != nil {
		return nil, err
	}
	return service.Unit(name)
}

// Initialize performs an initialization of the ZooKeeper nodes.
func (s *State) Initialize() error {
	stat, err := s.zk.Exists("/initialized")
	if err != nil {
		return err
	}
	if stat != nil {
		return nil
	}
	// Create new nodes.
	if _, err := s.zk.Create("/charms", "", 0, zkPermAll); err != nil {
		return err
	}
	if _, err := s.zk.Create("/services", "", 0, zkPermAll); err != nil {
		return err
	}
	if _, err := s.zk.Create("/machines", "", 0, zkPermAll); err != nil {
		return err
	}
	if _, err := s.zk.Create("/units", "", 0, zkPermAll); err != nil {
		return err
	}
	if _, err := s.zk.Create("/relations", "", 0, zkPermAll); err != nil {
		return err
	}
	// TODO Create node for bootstrap machine.

	// TODO Setup default global settings information.

	// Finally creation of /initialized as marker.
	if _, err := s.zk.Create("/initialized", "", 0, zkPermAll); err != nil {
		return err
	}
	return nil
}
