package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
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
	defer errorContextf(&err, "can't get the charm URL of service %q", s)
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
func (s *Service) SetCharmURL(url *charm.URL) (err error) {
	defer errorContextf(&err, "can't set the charm URL of service %q", s)
	cn, err := readConfigNode(s.st.zk, s.zkPath())
	if err != nil {
		return err
	}
	cn.Set("charm", url.String())
	_, err = cn.Write()
	return err
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
func (s *Service) addUnit(principalKey string) (unit *Unit, err error) {
	defer errorContextf(&err, "can't add unit to service %q", s)
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
		return nil, fmt.Errorf("can't add unit of principal service %q as a subordinate of %q", s, principal)
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
	defer errorContextf(&err, "can't get unit %q from service %q", name, s)
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
	defer errorContextf(&err, "can't get all units from service %q", s)
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
	defer errorContextf(&err, "can't get relations for service %q", s.name)
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
		return false, fmt.Errorf("can't check if service %q is exposed: %v", s, err)
	}
	return stat != nil, nil
}

// SetExposed marks the service as exposed.
// See ClearExposed and IsExposed.
func (s *Service) SetExposed() error {
	_, err := s.st.zk.Create(s.zkExposedPath(), "", 0, zkPermAll)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNODEEXISTS) {
		return fmt.Errorf("can't set exposed flag for service %q: %v", s, err)
	}
	return nil
}

// ClearExposed removes the exposed flag from the service.
// See SetExposed and IsExposed.
func (s *Service) ClearExposed() error {
	err := s.st.zk.Delete(s.zkExposedPath(), -1)
	if err != nil && !zookeeper.IsError(err, zookeeper.ZNONODE) {
		return fmt.Errorf("can't clear exposed flag for service %q: %v", s, err)
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
		return nil, fmt.Errorf("can't get configuration of service %q: %v", s, err)
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
	return &Unit{
		st:           s.st,
		key:          key,
		serviceName:  s.name,
		principalKey: principalKey,
	}
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
