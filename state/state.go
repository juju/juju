// The state package enables reading, observing, and changing
// the state stored in ZooKeeper of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/juju/go/charm"
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
	return &Machine{s, key}, nil
}

// RemoveMachine removes the machine with the given id.
func (s *State) RemoveMachine(id int) error {
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
	if err := retryTopologyChange(s.zk, removeMachine); err != nil {
		return fmt.Errorf("can't remove machine %d: %v", id, err)
	}
	return zkRemoveTree(s.zk, fmt.Sprintf("/machines/%s", key))
}

// WatchMachines watches for new Machines added or removed.
func (s *State) WatchMachines() *MachinesWatcher {
	return newMachinesWatcher(s)
}

// WatchEnvironConfig returns a watcher for observing
// changes to the environment configuration.
func (s *State) WatchEnvironConfig() *ConfigWatcher {
	return newConfigWatcher(s, zkEnvironmentPath)
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	key := machineKey(id)
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	if !topology.HasMachine(key) {
		return nil, fmt.Errorf("machine %d not found", id)
	}
	return &Machine{s, key}, nil
}

// AllMachines returns all machines in the environment.
func (s *State) AllMachines() ([]*Machine, error) {
	topology, err := readTopology(s.zk)
	if err != nil {
		return nil, err
	}
	machines := []*Machine{}
	for _, key := range topology.MachineKeys() {
		machines = append(machines, &Machine{s, key})
	}
	return machines, nil
}

// AddCharm adds the ch charm with curl to the state.
// bundleUrl must be set to a URL where the bundle for ch
// may be downloaded from.
// On success the newly added charm state is returned.
func (s *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (*Charm, error) {
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

// Charm returns a charm by the given id.
func (s *State) Charm(curl *charm.URL) (*Charm, error) {
	path, err := charmPath(curl)
	if err != nil {
		return nil, err
	}
	yaml, _, err := s.zk.Get(path)
	if zookeeper.IsError(err, zookeeper.ZNONODE) {
		return nil, fmt.Errorf("charm not found: %q", curl)
	}
	if err != nil {
		return nil, err
	}
	data := &charmData{}
	if err := goyaml.Unmarshal([]byte(yaml), data); err != nil {
		return nil, err
	}
	return newCharm(s, curl, data)
}

// AddService creates a new service state with the given unique name
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

// hasRelation checks if the given endpoints are already related.
func (s *State) hasRelation(t *topology, endpoints ...RelationEndpoint) (bool, error) {
	relationKey, err := t.RelationKey(endpoints...)
	if relationKey != "" {
		return true, nil
	}
	if _, ok := err.(*NoRelationError); ok {
		return false, nil
	}
	return false, err
}

// addRelation creates the relation node depending on the given scope.
func (s *State) addRelation(scope RelationScope) (string, error) {
	path, err := s.zk.Create("/relations/relation-", "", zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return "", err
	}
	relationKey := strings.Split(path, "/")[2]
	// Create the settings container, for individual units settings.
	// Creation is per container for container scoped relations and
	// occurs elsewhere.
	if scope == ScopeGlobal {
		_, err = s.zk.Create(path+"/settings", "", 0, zkPermAll)
		if err != nil {
			return "", err
		}
	}
	return relationKey, nil
}

// addRelationEndpoint creates the endpoint role node for the given relation
// and endpoint.
func (s *State) addServiceRelation(relationKey string, endpoint RelationEndpoint, scope RelationScope) error {
	// Creation is per container for container scoped relations and
	// occurs elsewhere.
	if scope == ScopeGlobal {
		path := fmt.Sprintf("/relations/%s/%s", relationKey, string(endpoint.RelationRole))
		_, err := s.zk.Create(path, "", 0, zkPermAll)
		if err != nil {
			return err
		}
	}
	return nil
}

// AddRelation creates a new relation state with the given endpoints.  
func (s *State) AddRelation(endpoints ...RelationEndpoint) (*Relation, []*ServiceRelation, error) {
	switch len(endpoints) {
	case 1:
		if endpoints[0].RelationRole != RolePeer {
			return nil, nil, fmt.Errorf("state: counterpart for %s endpoint is missing", endpoints[0].RelationRole)
		}
	case 2:
		if !endpoints[0].CanRelateTo(&endpoints[1]) {
			return nil, nil, fmt.Errorf("state: endpoints %s and %s are incompatible", endpoints[0], endpoints[1])
		}
	default:
		return nil, nil, fmt.Errorf("state: illegal number of endpoints provided")
	}
	// Check if the relation already exist.
	top, err := readTopology(s.zk)
	if err != nil {
		return nil, nil, err
	}
	alreadyAdded, err := s.hasRelation(top, endpoints...)
	if err != nil {
		return nil, nil, err
	}
	if alreadyAdded {
		return nil, nil, fmt.Errorf("state: relation has already been added")
	}
	// Add relation and service relations.
	scope := ScopeGlobal
	for _, endpoint := range endpoints {
		if endpoint.RelationScope == ScopeContainer {
			scope = ScopeContainer
			break
		}
	}
	relationKey, err := s.addRelation(scope)
	if err != nil {
		return nil, nil, err
	}
	serviceRelations := []*ServiceRelation{}
	for _, endpoint := range endpoints {
		serviceKey, err := top.ServiceKey(endpoint.ServiceName)
		if err != nil {
			return nil, nil, err
		}
		err = s.addServiceRelation(relationKey, endpoint, scope)
		if err != nil {
			return nil, nil, err
		}
		serviceRelations = append(serviceRelations, &ServiceRelation{
			st:         s,
			key:        relationKey,
			serviceKey: serviceKey,
			scope:      endpoint.RelationScope,
			role:       endpoint.RelationRole,
			name:       endpoint.RelationName,
		})
	}
	// Add relation to topology.
	addRelation := func(t *topology) error {
		relation := &zkRelation{
			Interface: endpoints[0].Interface,
			Scope:     scope,
			Services:  map[RelationRole]*zkRelationService{},
		}
		for _, serviceRelation := range serviceRelations {
			if !t.HasService(serviceRelation.serviceKey) {
				return fmt.Errorf("state: state for service %q has changed", serviceRelation.serviceKey)
			}
			service := &zkRelationService{
				ServiceKey:   serviceRelation.serviceKey,
				RelationName: serviceRelation.name,
			}
			relation.Services[serviceRelation.role] = service
		}
		return t.AddRelation(relationKey, relation)
	}
	err = retryTopologyChange(s.zk, addRelation)
	if err != nil {
		return nil, nil, err
	}
	return &Relation{s, relationKey}, serviceRelations, nil
}
