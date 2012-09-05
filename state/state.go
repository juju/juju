// The state package enables reading, observing, and changing
// the state stored in ZooKeeper of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/trivial"
	"net/url"
	"strings"
)

const (
	zkEnvironmentPath = "/environment"
	zkMachinesPath    = "/machines"
	zkTopologyPath    = "/topology"
)

// State represents the state of an environment
// managed by juju.
type State struct {
	zk  *zookeeper.Conn
	fwd *sshForwarder
}

// AddMachine creates a new machine state.
func (s *State) AddMachine() (m *Machine, err error) {
	defer trivial.ErrorContextf(&err, "cannot add a new machine")
	path, err := s.zk.Create("/machines/machine-", "", zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, err
	}
	key := strings.Split(path, "/")[2]
	addMachine := func(t *topology) error {
		return t.AddMachine(key)
	}
	if err = retryTopologyChange(s.zk, addMachine); err != nil {
		return
	}
	return newMachine(s, key), nil
}

// RemoveMachine removes the machine with the given id.
func (s *State) RemoveMachine(id int) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove machine %d", id)
	key := machineKey(id)
	removeMachine := func(t *topology) error {
		if !t.HasMachine(key) {
			return fmt.Errorf("machine not found")
		}
		hasUnits, err := t.MachineHasUnits(key)
		if err != nil {
			return err
		}
		if hasUnits {
			return fmt.Errorf("machine has units")
		}
		return t.RemoveMachine(key)
	}
	if err = retryTopologyChange(s.zk, removeMachine); err != nil {
		return
	}
	return zkRemoveTree(s.zk, fmt.Sprintf("/machines/%s", key))
}

// WatchServices returns a watcher for observing services
// being added or removed.
func (s *State) WatchServices() *ServicesWatcher {
	return newServicesWatcher(s)
}

// WatchMachines returns a watcher for observing machines
// being added or removed.
func (s *State) WatchMachines() *MachinesWatcher {
	return newMachinesWatcher(s)
}

// WatchEnvironConfig returns a watcher for observing
// changes to the environment configuration.
func (s *State) WatchEnvironConfig() *EnvironConfigWatcher {
	return newEnvironConfigWatcher(s)
}

// EnvironConfig returns the current configuration of the environment.
func (s *State) EnvironConfig() (*config.Config, error) {
	configNode, err := readConfigNode(s.zk, zkEnvironmentPath)
	if err != nil {
		return nil, err
	}
	attrs := configNode.Map()
	return config.New(attrs)
}

// SetEnvironConfig replaces the current configuration of the 
// environment with the passed configuration.
func (s *State) SetEnvironConfig(cfg *config.Config) error {
	attrs := cfg.AllAttrs()
	_, err := createConfigNode(s.zk, zkEnvironmentPath, attrs)
	return err
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	key := machineKey(id)
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, fmt.Errorf("cannot get machine %d: %v", id, err)
	}
	if !topology.HasMachine(key) {
		return nil, fmt.Errorf("machine %d not found", id)
	}
	return newMachine(s, key), nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() ([]*Machine, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, fmt.Errorf("cannot get all machines: %v", err)
	}
	machines := []*Machine{}
	for _, key := range topology.MachineKeys() {
		machines = append(machines, newMachine(s, key))
	}
	return machines, nil
}

// AddCharm adds the ch charm with curl to the state.
// bundleUrl must be set to a URL where the bundle for ch
// may be downloaded from.
// On success the newly added charm state is returned.
func (s *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (stch *Charm, err error) {
	defer trivial.ErrorContextf(&err, "cannot add charm %q", curl)
	data := &charmData{
		Meta:         ch.Meta(),
		Config:       ch.Config(),
		BundleURL:    bundleURL.String(),
		BundleSha256: bundleSha256,
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
	return newCharm(s, curl, data)
}

// Charm returns the charm with the given URL.
func (s *State) Charm(curl *charm.URL) (stch *Charm, err error) {
	defer trivial.ErrorContextf(&err, "cannot get charm %q", curl)
	path, err := charmPath(curl)
	if err != nil {
		return
	}
	yaml, _, err := s.zk.Get(path)
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		return nil, fmt.Errorf("charm not found")
	}
	if err != nil {
		return nil, err
	}
	data := &charmData{}
	if err = goyaml.Unmarshal([]byte(yaml), data); err != nil {
		return nil, err
	}
	return newCharm(s, curl, data)
}

// AddService creates a new service state with the given unique name
// and the charm state.
func (s *State) AddService(name string, ch *Charm) (service *Service, err error) {
	defer trivial.ErrorContextf(&err, "cannot add service %q", name)
	initial := map[string]interface{}{
		"charm":       ch.URL().String(),
		"force-charm": false,
	}
	yaml, err := goyaml.Marshal(initial)
	if err != nil {
		return nil, err
	}
	path, err := s.zk.Create("/services/service-", string(yaml), zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, err
	}
	key := strings.Split(path, "/")[2]
	service = &Service{s, key, name}
	// Create an empty configuration node.
	_, err = createConfigNode(s.zk, service.zkConfigPath(), map[string]interface{}{})
	if err != nil {
		return nil, err
	}
	// Create a parent node for the service units
	_, err = s.zk.Create(service.zkPath()+"/units", "", 0, zkPermAll)
	if err != nil {
		return nil, err
	}
	addService := func(t *topology) error {
		if _, err := t.ServiceKey(name); err == nil {
			// No error, so service name already in use.
			return fmt.Errorf("service name is already in use")
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
func (s *State) RemoveService(svc *Service) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove service %q", svc.Name())
	// Remove relations first, to minimize unwanted hook executions.
	rels, err := svc.Relations()
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if err := s.RemoveRelation(rel); err != nil {
			return err
		}
	}
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
		return t.RemoveService(svc.key)
	}
	if err = retryTopologyChange(s.zk, removeService); err != nil {
		return err
	}
	return zkRemoveTree(s.zk, svc.zkPath())
}

// Service returns the service with the given name.
func (s *State) Service(name string) (service *Service, err error) {
	defer trivial.ErrorContextf(&err, "cannot get service %q", name)
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
func (s *State) AllServices() (services []*Service, err error) {
	defer trivial.ErrorContextf(&err, "cannot get all services")
	topology, err := readTopology(s.zk)
	if err != nil {
		return
	}
	services = []*Service{}
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
func (s *State) Unit(name string) (unit *Unit, err error) {
	defer trivial.ErrorContextf(&err, "cannot get unit %q", name)
	serviceName, _, err := parseUnitName(name)
	if err != nil {
		return
	}
	service, err := s.Service(serviceName)
	if err != nil {
		return
	}
	return service.Unit(name)
}

// AssignUnit places the unit on a machine. Depending on the policy, and the
// state of the environment, this may lead to new instances being launched
// within the environment.
func (s *State) AssignUnit(u *Unit, policy AssignmentPolicy) (err error) {
	if !u.IsPrincipal() {
		return fmt.Errorf("subordinate unit %q cannot be assigned directly to a machine", u)
	}
	defer trivial.ErrorContextf(&err, "cannot assign unit %q to machine", u)
	var m *Machine
	switch policy {
	case AssignLocal:
		if m, err = s.Machine(0); err != nil {
			return err
		}
	case AssignUnused:
		if _, err = u.AssignToUnusedMachine(); err != noUnusedMachines {
			return err
		}
		if m, err = s.AddMachine(); err != nil {
			return err
		}
	default:
		panic(fmt.Errorf("unknown unit assignment policy: %q", policy))
	}
	return u.AssignToMachine(m)
}

// addRelationNode creates the node for the relation represented by the
// given endpoints, and returns the node name to be used as a relation key.
// The provided endpoints are validated before the relation node is created.
func (s *State) addRelationNode(endpoints ...RelationEndpoint) (relationKey string, err error) {
	switch len(endpoints) {
	case 1:
		if endpoints[0].RelationRole != RolePeer {
			return "", fmt.Errorf("single endpoint must be a peer relation")
		}
	case 2:
		if !endpoints[0].CanRelateTo(&endpoints[1]) {
			return "", fmt.Errorf("endpoints do not relate")
		}
	default:
		return "", fmt.Errorf("cannot relate %d endpoints", len(endpoints))
	}
	t, err := readTopology(s.zk)
	if err != nil {
		return
	}
	// Check if the relation already exists.
	_, err = t.RelationKey(endpoints...)
	if err == nil {
		return "", fmt.Errorf("relation already exists")
	}
	if err != noRelationFound {
		return
	}
	// Add the node.
	path, err := s.zk.Create("/relations/relation-", "", zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return
	}
	relationKey = strings.Split(path, "/")[2]
	return
}

// AddRelation creates a new relation with the given endpoints.
func (s *State) AddRelation(endpoints ...RelationEndpoint) (rel *Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot add relation %q", describeEndpoints(endpoints))
	key, err := s.addRelationNode(endpoints...)
	if err != nil {
		return nil, err
	}
	err = retryTopologyChange(s.zk, func(t *topology) error {
		relation := &topoRelation{
			Interface: endpoints[0].Interface,
			Scope:     charm.ScopeGlobal,
		}
		for _, endpoint := range endpoints {
			if endpoint.RelationScope == charm.ScopeContainer {
				relation.Scope = charm.ScopeContainer
			}
			serviceKey, err := t.ServiceKey(endpoint.ServiceName)
			if err != nil {
				return err
			}
			tendpoint := topoEndpoint{
				serviceKey, endpoint.RelationRole, endpoint.RelationName,
			}
			relation.Endpoints = append(relation.Endpoints, tendpoint)
		}
		return t.AddRelation(key, relation)
	})
	if err != nil {
		return nil, err
	}
	return s.Relation(endpoints...)
}

// Relation returns the existing relation with the given endpoints.
func (s *State) Relation(endpoints ...RelationEndpoint) (r *Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot get relation %q", describeEndpoints(endpoints))
	t, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	key, err := t.RelationKey(endpoints...)
	if err != nil {
		return nil, err
	}
	tr, err := t.Relation(key)
	if err != nil {
		return nil, err
	}
	r = &Relation{s, key, nil}
	for _, tep := range tr.Endpoints {
		sname, err := t.ServiceName(tep.Service)
		if err != nil {
			return nil, err
		}
		r.endpoints = append(r.endpoints, RelationEndpoint{
			sname, tr.Interface, tep.RelationName, tep.RelationRole, tr.Scope,
		})
	}
	return r, nil
}

// RemoveRelation removes the supplied relation.
func (s *State) RemoveRelation(r *Relation) error {
	err := retryTopologyChange(s.zk, func(t *topology) error {
		if !t.HasRelation(r.key) {
			return fmt.Errorf("not found")
		}
		return t.RemoveRelation(r.key)
	})
	if err != nil {
		return fmt.Errorf("cannot remove relation %q: %s", r, err)
	}
	return nil
}
