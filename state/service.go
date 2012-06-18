// launchpad.net/juju/state
//
// Copyright (c) 2011-2012 Canonical Ltd.

package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/juju/charm"
	pathPkg "path"
)

// Service represents the state of a service.
type Service struct {
	st   *State
	key  string
	name string
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.name
}

// CharmURL returns the charm URL this service is supposed
// to use.
func (s *Service) CharmURL() (url *charm.URL, err error) {
	cn, err := readConfigNode(s.st.zk, s.zkPath())
	if err != nil {
		return nil, err
	}
	if id, ok := cn.Get("charm"); ok {
		url, err = charm.ParseURL(id.(string))
		if err != nil {
			return nil, err
		}
		return url, nil
	}
	return nil, errors.New("service has no charm URL")
}

// SetCharmURL changes the charm URL for the service.
func (s *Service) SetCharmURL(url *charm.URL) error {
	cn, err := readConfigNode(s.st.zk, s.zkPath())
	if err != nil {
		return err
	}
	cn.Set("charm", url.String())
	_, err = cn.Write()
	if err != nil {
		return err
	}
	return nil
}

// Charm returns the service's charm.
func (s *Service) Charm() (*Charm, error) {
	url, err := s.CharmURL()
	if err != nil {
		return nil, err
	}
	return s.st.Charm(url)
}

// addUnit adds a new unit to the service. If s is a subordinate service,
// principalKey must be the unit key of some principal unit.
func (s *Service) addUnit(principalKey string) (*Unit, error) {
	// Get charm id and create ZooKeeper node.
	url, err := s.CharmURL()
	if err != nil {
		return nil, err
	}
	unitData := map[string]string{"charm": url.String()}
	unitYaml, err := goyaml.Marshal(unitData)
	if err != nil {
		return nil, err
	}
	keyPrefix := s.zkPath() + "/units/unit-" + s.key[len("service-"):] + "-"
	path, err := s.st.zk.Create(keyPrefix, string(unitYaml), zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, err
	}
	key := pathPkg.Base(path)
	addUnit := func(t *topology) error {
		if !t.HasService(s.key) {
			return stateChanged
		}
		err := t.AddUnit(key, principalKey)
		if err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.st.zk, addUnit); err != nil {
		return nil, err
	}
	return &Unit{
		st:          s.st,
		key:         key,
		serviceName: s.name,
		isPrincipal: principalKey == "",
	}, nil
}

// AddUnit adds a new principal unit to the service.
func (s *Service) AddUnit() (*Unit, error) {
	ch, err := s.Charm()
	if err != nil {
		return nil, err
	}
	if ch.Meta().Subordinate {
		return nil, fmt.Errorf("cannot directly add units to subordinate service %q", s.name)
	}
	return s.addUnit("")
}

// AddUnitSubordinateTo adds a new subordinate unit to the service,
// subordinate to principal.
func (s *Service) AddUnitSubordinateTo(principal *Unit) (*Unit, error) {
	ch, err := s.Charm()
	if err != nil {
		return nil, err
	}
	if !ch.Meta().Subordinate {
		return nil, errors.New("cannot make a principal unit subordinate to another unit")
	}
	ok := principal.IsPrincipal()
	if !ok {
		return nil, errors.New("a subordinate unit must be added to a principal unit")
	}
	return s.addUnit(principal.key)
}

// RemoveUnit() removes a unit.
func (s *Service) RemoveUnit(unit *Unit) error {
	// First unassign from machine if currently assigned.
	if err := unit.UnassignFromMachine(); err != nil {
		return err
	}
	removeUnit := func(t *topology) error {
		if !t.HasUnit(unit.key) {
			return stateChanged
		}
		if err := t.RemoveUnit(unit.key); err != nil {
			return err
		}
		return nil
	}
	if err := retryTopologyChange(s.st.zk, removeUnit); err != nil {
		return err
	}
	return zkRemoveTree(s.st.zk, unit.zkPath())
}

// Unit returns the service's unit with name.
func (s *Service) Unit(name string) (*Unit, error) {
	serviceName, serviceId, err := parseUnitName(name)
	if err != nil {
		return nil, err
	}
	// Check for matching service name.
	if serviceName != s.name {
		return nil, fmt.Errorf("can't find unit %q on service %q", name, s.name)
	}
	topology, err := readTopology(s.st.zk)
	if err != nil {
		return nil, err
	}
	if !topology.HasService(s.key) {
		return nil, stateChanged
	}

	// Check that unit exists.
	key := makeUnitKey(s.key, serviceId)
	_, tunit, err := topology.serviceAndUnit(key)
	if err != nil {
		return nil, err
	}
	return &Unit{
		st:          s.st,
		key:         key,
		serviceName: s.name,
		isPrincipal: tunit.isPrincipal(),
	}, nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() ([]*Unit, error) {
	topology, err := readTopology(s.st.zk)
	if err != nil {
		return nil, err
	}
	if !topology.HasService(s.key) {
		return nil, stateChanged
	}
	keys, err := topology.UnitKeys(s.key)
	if err != nil {
		return nil, err
	}
	// Assemble units.
	units := []*Unit{}
	for _, key := range keys {
		_, tunit, err := topology.serviceAndUnit(key)
		if err != nil {
			return nil, fmt.Errorf("inconsistent topology: %v", err)
		}
		units = append(units, &Unit{
			st:          s.st,
			key:         key,
			serviceName: s.name,
			isPrincipal: tunit.isPrincipal(),
		})
	}
	return units, nil
}

// Relations returns a ServiceRelation for every relation the service is in.
func (s *Service) Relations() ([]*ServiceRelation, error) {
	var err error
	defer errorContextf(&err, "can't get relations for service %q", s.name)
	t, err := readTopology(s.st.zk)
	if err != nil {
		return nil, err
	}
	relations, err := t.RelationsForService(s.key)
	if err != nil {
		return nil, err
	}
	serviceRelations := []*ServiceRelation{}
	for key, relation := range relations {
		rs := relation.Services[s.key]
		serviceRelations = append(serviceRelations, &ServiceRelation{
			s.st, key, s.key, relation.Scope, rs.RelationRole, rs.RelationName,
		})
	}
	return serviceRelations, nil
}

// IsExposed returns whether this service is exposed.
// The explicitly open ports (with open-port) for exposed
// services may be accessed from machines outside of the
// local deployment network. See SetExposed and ClearExposed.
func (s *Service) IsExposed() (bool, error) {
	stat, err := s.st.zk.Exists(s.zkExposedPath())
	if err != nil {
		return false, err
	}
	return stat != nil, nil
}

// SetExposed marks the service as exposed.
// See ClearExposed and IsExposed.
func (s *Service) SetExposed() error {
	_, err := s.st.zk.Create(s.zkExposedPath(), "", 0, zkPermAll)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
		return err
	}
	return nil
}

// ClearExposed removes the exposed flag from the service.
// See SetExposed and IsExposed.
func (s *Service) ClearExposed() error {
	err := s.st.zk.Delete(s.zkExposedPath(), -1)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		return err
	}
	return nil
}

// Config returns the configuration node for the service.
func (s *Service) Config() (*ConfigNode, error) {
	return readConfigNode(s.st.zk, s.zkConfigPath())
}

// WatchConfig creates a watcher for the configuration node
// of the service.
func (s *Service) WatchConfig() *ConfigWatcher {
	return newConfigWatcher(s.st, s.zkConfigPath())
}

// zkPath returns the ZooKeeper base path for the service.
func (s *Service) zkPath() string {
	return fmt.Sprintf("/services/%s", s.key)
}

// zkConfigPath returns the ZooKeeper path for the service configuration.
func (s *Service) zkConfigPath() string {
	return s.zkPath() + "/config"
}

// zkExposedPath, if exists in ZooKeeper, indicates, that a
// service is exposed.
func (s *Service) zkExposedPath() string {
	return s.zkPath() + "/exposed"
}
