package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/trivial"
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

// serviceNode defines the service node serialization.
type serviceNode struct {
	CharmURL   string `yaml:"charm-url"`
	ForceCharm bool   `yaml:"force-charm,omitempty"`
}

// ServiceCharm describes the charm that units of the service should use.
// If a unit's charm differs from the service's, the unit should upgrade.
type ServiceCharm struct {
	*Charm
	// Force indicates whether units should upgrade to this charm even
	// if they are in an error state, which would usually block upgrades.
	Force bool
}

// readServiceCharm unmarshals yaml into a ServiceCharm.
func readServiceCharm(st *State, yaml string) (sc ServiceCharm, err error) {
	var sn serviceNode
	if err = goyaml.Unmarshal([]byte(yaml), &sn); err != nil {
		return
	}
	url, err := charm.ParseURL(sn.CharmURL)
	if err != nil {
		return
	}
	ch, err := st.Charm(url)
	if err != nil {
		return
	}
	return ServiceCharm{ch, sn.ForceCharm}, nil
}

// Charm returns the service's charm, and whether units should upgrade to that
// charm even if they are in an error state.
func (s *Service) Charm() (sc ServiceCharm, err error) {
	defer trivial.ErrorContextf(&err, "cannot get charm for service %q", s)
	yaml, _, err := s.st.zk.Get(s.zkPath())
	if err != nil {
		return
	}
	return readServiceCharm(s.st, yaml)
}

// SetCharm changes the charm for the service. New units will be started with
// this charm, and existing units will be upgraded to use it. If force is true,
// units will be upgraded even if they are in an error state.
func (s *Service) SetCharm(ch *Charm, force bool) (err error) {
	defer trivial.ErrorContextf(&err, "cannot set charm for service %q", s)
	setCharm := func(oldYaml string, _ *zookeeper.Stat) (string, error) {
		var sn serviceNode
		if err = goyaml.Unmarshal([]byte(oldYaml), &sn); err != nil {
			return "", err
		}
		url := ch.URL().String()
		if sn.CharmURL == url && sn.ForceCharm == force {
			return oldYaml, nil
		}
		sn.CharmURL = url
		sn.ForceCharm = force
		newYaml, err := goyaml.Marshal(&sn)
		if err != nil {
			return "", err
		}
		return string(newYaml), nil
	}
	return s.st.zk.RetryChange(s.zkPath(), 0, zkPermAll, setCharm)
}

// WatchCharm returns a watcher that sends notifications of changes to the
// service's charm.
func (s *Service) WatchCharm() *ServiceCharmWatcher {
	return newServiceCharmWatcher(s.st, s.zkPath())
}

// addUnit adds a new unit to the service. If s is a subordinate service,
// principalKey must be the unit key of some principal unit.
func (s *Service) addUnit(principalKey string) (unit *Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot add unit to service %q", s)
	// Create ZooKeeper node.
	keyPrefix := s.zkPath() + "/units/unit-" + s.key[len("service-"):] + "-"
	path, err := s.st.zk.Create(keyPrefix, "", zookeeper.SEQUENCE, zkPermAll)
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
	return s.newUnit(key, principalKey), nil
}

// AddUnit adds a new principal unit to the service.
func (s *Service) AddUnit() (*Unit, error) {
	ch, err := s.Charm()
	if err != nil {
		return nil, err
	}
	if ch.Meta().Subordinate {
		return nil, fmt.Errorf("cannot directly add units to subordinate service %q", s)
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
		return nil, fmt.Errorf("cannot add unit of principal service %q as a subordinate of %q", s, principal)
	}
	if !principal.IsPrincipal() {
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
			return fmt.Errorf("unit not found")
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
func (s *Service) Unit(name string) (unit *Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot get unit %q from service %q", name, s)
	serviceName, serviceId, err := parseUnitName(name)
	if err != nil {
		return nil, err
	}
	// Check for matching service name.
	if serviceName != s.name {
		return nil, fmt.Errorf("unit not found")
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
	return s.newUnit(key, tunit.Principal), nil
}

// AllUnits returns all units of the service.
func (s *Service) AllUnits() (units []*Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot get all units from service %q", s)
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
	units = []*Unit{}
	for _, key := range keys {
		_, tunit, err := topology.serviceAndUnit(key)
		if err != nil {
			return nil, fmt.Errorf("inconsistent topology: %v", err)
		}
		units = append(units, s.newUnit(key, tunit.Principal))
	}
	return units, nil
}

// WatchUnits creates a watcher for the assigned units
// of the service.
func (s *Service) WatchUnits() *ServiceUnitsWatcher {
	return newServiceUnitsWatcher(s)
}

// relationsFromTopology returns a Relation for every relation the service
// is in, according to the supplied topology.
func (s *Service) relationsFromTopology(t *topology) ([]*Relation, error) {
	if !t.HasService(s.key) {
		return nil, stateChanged
	}
	trs, err := t.RelationsForService(s.key)
	if err != nil {
		return nil, err
	}
	relations := []*Relation{}
	for key, tr := range trs {
		r := &Relation{s.st, key, make([]RelationEndpoint, len(tr.Endpoints))}
		i := 0
		for _, tep := range tr.Endpoints {
			sname := s.name
			if tep.Service != s.key {
				if sname, err = t.ServiceName(tep.Service); err != nil {
					return nil, err
				}
			}
			r.endpoints[i] = RelationEndpoint{
				sname, tr.Interface, tep.RelationName, tep.RelationRole, tr.Scope,
			}
			i++
		}
		relations = append(relations, r)
	}
	return relations, nil
}

// Relations returns a Relation for every relation the service is in.
func (s *Service) Relations() (relations []*Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot get relations for service %q", s.name)
	t, err := readTopology(s.st.zk)
	if err != nil {
		return nil, err
	}
	return s.relationsFromTopology(t)
}

// WatchRelations returns a watcher which notifies of changes to the
// set of relations in which this service is participating.
func (s *Service) WatchRelations() *ServiceRelationsWatcher {
	return newServiceRelationsWatcher(s)
}

// IsExposed returns whether this service is exposed.
// The explicitly open ports (with open-port) for exposed
// services may be accessed from machines outside of the
// local deployment network. See SetExposed and ClearExposed.
func (s *Service) IsExposed() (bool, error) {
	stat, err := s.st.zk.Exists(s.zkExposedPath())
	if err != nil {
		return false, fmt.Errorf("cannot check if service %q is exposed: %v", s, err)
	}
	return stat != nil, nil
}

// SetExposed marks the service as exposed.
// See ClearExposed and IsExposed.
func (s *Service) SetExposed() error {
	_, err := s.st.zk.Create(s.zkExposedPath(), "", 0, zkPermAll)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
		return fmt.Errorf("cannot set exposed flag for service %q: %v", s, err)
	}
	return nil
}

// ClearExposed removes the exposed flag from the service.
// See SetExposed and IsExposed.
func (s *Service) ClearExposed() error {
	err := s.st.zk.Delete(s.zkExposedPath(), -1)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		return fmt.Errorf("cannot clear exposed flag for service %q: %v", s, err)
	}
	return nil
}

// WatchExposed creates a watcher for the exposed flag
// of the service.
func (s *Service) WatchExposed() *FlagWatcher {
	return newFlagWatcher(s.st, s.zkExposedPath())
}

// Config returns the configuration node for the service.
func (s *Service) Config() (config *ConfigNode, err error) {
	config, err = readConfigNode(s.st.zk, s.zkConfigPath())
	if err != nil {
		return nil, fmt.Errorf("cannot get configuration of service %q: %v", s, err)
	}
	return config, nil
}

// WatchConfig creates a watcher for the configuration node
// of the service.
func (s *Service) WatchConfig() *ConfigWatcher {
	return newConfigWatcher(s.st, s.zkConfigPath())
}

// String returns the service name.
func (s *Service) String() string {
	return s.Name()
}

// newUnit creates a *Unit.
func (s *Service) newUnit(key, principalKey string) *Unit {
	return newUnit(s.st, s.name, key, principalKey)
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
