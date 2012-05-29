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
func (s *State) WatchEnvrionConfig() *ConfigWatcher {
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

// AddClientServerRelation adds a relation between a client and a server endpoint.
// Their corresponding services will be assigned automatically.
func (s *State) AddClientServerRelation(clientEp, serverEp RelationEndpoint) (*Relation, []*ServiceRelation, error) {
	if clientEp.RelationRole != RoleClient {
		return nil, nil, fmt.Errorf("interface %q of service %q has not the client role",
			clientEp.Interface, clientEp.ServiceName)
	}
	if serverEp.RelationRole != RoleServer {
		return nil, nil, fmt.Errorf("interface %q of service %q has not the server role",
			serverEp.Interface, serverEp.ServiceName)
	}
	if clientEp.Interface != serverEp.Interface {
		return nil, nil, fmt.Errorf("client and server endpoints have different interfaces")
	}
	top, err := readTopology(s.zk)
	if err != nil {
		return nil, nil, err
	}
	relationKey, err := top.RelationKey(clientEp, serverEp)
	if relationKey != "" {
		return nil, nil, fmt.Errorf("client and server already have relation %q", relationKey)
	}
	if err != nil && err != errRelationDoesNotExist {
		return nil, nil, err
	}
	scope := ScopeGlobal
	if clientEp.RelationScope == ScopeContainer || serverEp.RelationScope == ScopeContainer {
		scope = ScopeContainer
	}
	path, err := s.zk.Create("/relations/relation-", "", zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, nil, err
	}
	relationKey = strings.Split(path, "/")[2]
	// Create the settings container, for individual units settings.
	// Creation is per container for container scoped relations and
	// occurs elsewhere.
	if scope == ScopeGlobal {
		_, err = s.zk.Create(path+"/settings", "", 0, zkPermAll)
		if err != nil {
			return nil, nil, err
		}
	}
	serviceRelations := []*ServiceRelation{}
	// createNode creates the ZooKeeper node for an endpoint. The full path is
	// /relations/relationKey/optionalContainerKey/relationRole/...
	// How far down the path we can create at this point depends on
	// what scope the relation has.
	createNode := func(ep RelationEndpoint) (string, error) {
		serviceKey, err := top.ServiceKey(ep.ServiceName)
		if err != nil {
			return "", err
		}
		if scope == ScopeGlobal {
			_, err = s.zk.Create(path+"/"+string(ep.RelationRole), "", 0, zkPermAll)
			if err != nil {
				return "", err
			}
		}
		serviceRelations = append(serviceRelations, &ServiceRelation{
			st:         s,
			key:        relationKey,
			serviceKey: serviceKey,
			scope:      ep.RelationScope,
			role:       ep.RelationRole,
		})
		return serviceKey, nil
	}
	clientKey, err := createNode(clientEp)
	if err != nil {
		return nil, nil, err
	}
	serverKey, err := createNode(serverEp)
	if err != nil {
		return nil, nil, err
	}
	addRelation := func(t *topology) error {
		return t.AddClientServerRelation(relationKey, clientKey, serverKey,
			serverEp.Interface, serverEp.RelationScope)
	}
	err = retryTopologyChange(s.zk, addRelation)
	if err != nil {
		return nil, nil, err
	}
	return &Relation{s, relationKey}, serviceRelations, nil
}

// AddPeerRelation adds a relation with the peer endpoint.
// Its corresponding service will be assigned automatically.
func (s *State) AddPeerRelation(peerEp RelationEndpoint) (*Relation, *ServiceRelation, error) {
	if peerEp.RelationRole != RolePeer {
		return nil, nil, fmt.Errorf("interface %q of service %q has not the peer role",
			peerEp.Interface, peerEp.ServiceName)
	}
	top, err := readTopology(s.zk)
	if err != nil {
		return nil, nil, err
	}
	relationKey, err := top.PeerRelationKey(peerEp)
	if relationKey != "" {
		return nil, nil, fmt.Errorf("peer already has relation %q", relationKey)
	}
	if err != nil && err != errRelationDoesNotExist {
		return nil, nil, err
	}
	scope := ScopeGlobal
	if peerEp.RelationScope == ScopeContainer {
		scope = ScopeContainer
	}
	path, err := s.zk.Create("/relations/relation-", "", zookeeper.SEQUENCE, zkPermAll)
	if err != nil {
		return nil, nil, err
	}
	relationKey = strings.Split(path, "/")[2]
	// Create the settings container, for individual units settings.
	// Creation is per container for container scoped relations and
	// occurs elsewhere.
	if scope == ScopeGlobal {
		_, err = s.zk.Create(path+"/settings", "", 0, zkPermAll)
		if err != nil {
			return nil, nil, err
		}
	}
	// Create the ZooKeeper node for the endpoint. The full path is
	// /relations/relationKey/optionalContainerKey/relationRole/...
	// How far down the path we can create at this point depends on
	// what scope the relation has.
	peerKey, err := top.ServiceKey(peerEp.ServiceName)
	if err != nil {
		return nil, nil, err
	}
	if scope == ScopeGlobal {
		_, err = s.zk.Create(path+"/"+string(peerEp.RelationRole), "", 0, zkPermAll)
		if err != nil {
			return nil, nil, err
		}
	}
	serviceRelation := &ServiceRelation{
		st:         s,
		key:        relationKey,
		serviceKey: peerKey,
		scope:      peerEp.RelationScope,
		role:       peerEp.RelationRole,
	}
	addRelation := func(t *topology) error {
		return t.AddPeerRelation(relationKey, peerKey, peerEp.Interface, peerEp.RelationScope)
	}
	err = retryTopologyChange(s.zk, addRelation)
	if err != nil {
		return nil, nil, err
	}
	return &Relation{s, relationKey}, serviceRelation, nil
}
