package state

import (
	"errors"
	"fmt"
	"launchpad.net/goyaml"
	"launchpad.net/gozk/zookeeper"
	"sort"
)

// The protocol version, which is stored in the /topology node under
// the "version" key. The protocol version should *only* be updated
// when we know that a version is in fact actually incompatible.

const topologyVersion = 1

// zkTopology is used to marshal and unmarshal the content
// of the /topology node in ZooKeeper.
type zkTopology struct {
	Version      int
	Machines     map[string]*zkMachine
	Services     map[string]*zkService
	UnitSequence map[string]int "unit-sequence"
	Relations    map[string]*zkRelation
}

// zkMachine represents the machine data within the /topology
// node in ZooKeeper.
type zkMachine struct {
}

// zkService represents the service data within the /topology
// node in ZooKeeper.
type zkService struct {
	Name  string
	Units map[string]*zkUnit
}

// zkUnit represents the unit data within the /topology
// node in ZooKeeper.
type zkUnit struct {
	Sequence int
	Machine  string
}

// zkRelation represents the relation data within the 
// /topology node in ZooKeeper. "Members" references to
// the service keys of server and client or a peer.
type zkRelation struct {
	Interface string
	Scope     RelationScope
	Members   map[RelationRole]string
}

// topology is an internal helper that handles the content
// of the /topology node in ZooKeeper.
type topology struct {
	topology *zkTopology
}

// readTopology connects ZooKeeper, retrieves the data as YAML,
// parses it and returns it.
func readTopology(zk *zookeeper.Conn) (*topology, error) {
	yaml, _, err := zk.Get("/topology")
	if err != nil {
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			// No topology node, so return empty topology.
			return parseTopology("")
		}
		return nil, err
	}
	return parseTopology(yaml)
}

// dump returns the topology as YAML.
func (t *topology) dump() (string, error) {
	yaml, err := goyaml.Marshal(t.topology)
	if err != nil {
		return "", err
	}
	return string(yaml), nil
}

// Version returns the version of the topology.
func (t *topology) Version() int {
	return t.topology.Version
}

// AddMachine adds a new machine to the topology.
func (t *topology) AddMachine(key string) error {
	if t.topology.Machines == nil {
		t.topology.Machines = make(map[string]*zkMachine)
	} else if t.HasMachine(key) {
		return fmt.Errorf("attempted to add duplicated machine %q", key)
	}
	t.topology.Machines[key] = &zkMachine{}
	return nil
}

// RemoveMachine removes the machine with key from the topology.
func (t *topology) RemoveMachine(key string) error {
	ok, err := t.MachineHasUnits(key)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("can't remove machine %q while units ared assigned", key)
	}
	// Machine exists and has no units, so remove it.
	delete(t.topology.Machines, key)
	return nil
}

// MachineKeys returns all machine keys.
func (t *topology) MachineKeys() []string {
	keys := []string{}
	for key, _ := range t.topology.Machines {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// HasMachine returns whether a machine with key exists.
func (t *topology) HasMachine(key string) bool {
	return t.topology.Machines[key] != nil
}

// MachineHasUnits returns whether the machine with key has any units assigned to it.
func (t *topology) MachineHasUnits(key string) (bool, error) {
	err := t.assertMachine(key)
	if err != nil {
		return false, err
	}
	for _, service := range t.topology.Services {
		for _, unit := range service.Units {
			if unit.Machine == key {
				return true, nil
			}
		}
	}
	return false, nil
}

// AddService adds a new service to the topology.
func (t *topology) AddService(key, name string) error {
	if t.topology.Services == nil {
		t.topology.Services = make(map[string]*zkService)
	}
	if t.HasService(key) {
		return fmt.Errorf("attempted to add duplicated service %q", key)
	}
	if _, err := t.ServiceKey(name); err == nil {
		return fmt.Errorf("service name %q already in use", name)
	}
	t.topology.Services[key] = &zkService{
		Name:  name,
		Units: make(map[string]*zkUnit),
	}
	if t.topology.UnitSequence == nil {
		t.topology.UnitSequence = make(map[string]int)
	}
	if _, ok := t.topology.UnitSequence[name]; !ok {
		t.topology.UnitSequence[name] = 0
	}
	return nil
}

// RemoveService removes a service from the topology.
func (t *topology) RemoveService(key string) error {
	if err := t.assertService(key); err != nil {
		return err
	}
	infos, err := t.ActiveServiceEndpoints(key)
	if err != nil {
		return err
	}
	if len(infos) > 0 {
		return fmt.Errorf("cannot remove service %q with active relations", key)
	}
	delete(t.topology.Services, key)
	return nil
}

// HasService returns true if a service with the given key exists.
func (t *topology) HasService(key string) bool {
	return t.topology.Services[key] != nil
}

// ServiceKey returns the key of the service with the given name.
func (t *topology) ServiceKey(name string) (string, error) {
	for key, svc := range t.topology.Services {
		if svc.Name == name {
			return key, nil
		}
	}
	return "", fmt.Errorf("service with name %q not found", name)
}

// ServiceKeys returns all service keys.
func (t *topology) ServiceKeys() []string {
	keys := []string{}
	for key, _ := range t.topology.Services {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ServiceName returns the name of the service with the given key.
func (t *topology) ServiceName(key string) (string, error) {
	if svc, ok := t.topology.Services[key]; ok {
		return svc.Name, nil
	}
	return "", fmt.Errorf("service with key %q not found", key)
}

// HasUnit returns true if a unit with given service and unit keys exists.
func (t *topology) HasUnit(serviceKey, unitKey string) bool {
	if t.HasService(serviceKey) {
		return t.topology.Services[serviceKey].Units[unitKey] != nil
	}
	return false
}

// AddUnit adds a new unit and returns the sequence number. This
// sequence number will be increased monotonically for each service.
func (t *topology) AddUnit(serviceKey, unitKey string) (int, error) {
	if err := t.assertService(serviceKey); err != nil {
		return -1, err
	}
	// Check if unit key is unused.
	for key, svc := range t.topology.Services {
		if _, ok := svc.Units[unitKey]; ok {
			return -1, fmt.Errorf("unit %q already in use in service %q", unitKey, key)
		}
	}
	// Add unit and increase sequence number.
	svc := t.topology.Services[serviceKey]
	sequenceNo := t.topology.UnitSequence[svc.Name]
	svc.Units[unitKey] = &zkUnit{Sequence: sequenceNo}
	t.topology.UnitSequence[svc.Name] += 1
	return sequenceNo, nil
}

// RemoveUnit removes a unit from a service.
func (t *topology) RemoveUnit(serviceKey, unitKey string) error {
	if err := t.assertUnit(serviceKey, unitKey); err != nil {
		return err
	}
	delete(t.topology.Services[serviceKey].Units, unitKey)
	return nil
}

// UnitKeys returns the unit keys for all units of
// the service with the given service key in alphabetical order.
func (t *topology) UnitKeys(serviceKey string) ([]string, error) {
	if err := t.assertService(serviceKey); err != nil {
		return nil, err
	}
	keys := []string{}
	svc := t.topology.Services[serviceKey]
	for key, _ := range svc.Units {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// UnitName returns the name of a unit by its service key and its own key.
func (t *topology) UnitName(serviceKey, unitKey string) (string, error) {
	if err := t.assertUnit(serviceKey, unitKey); err != nil {
		return "", err
	}
	svc := t.topology.Services[serviceKey]
	unit := svc.Units[unitKey]
	return fmt.Sprintf("%s/%d", svc.Name, unit.Sequence), nil
}

// UnitKeyFromSequence returns the key of a unit based on its service key
// and its sequence number.
func (t *topology) UnitKeyFromSequence(serviceKey string, sequenceNo int) (string, error) {
	if err := t.assertService(serviceKey); err != nil {
		return "", err
	}
	svc := t.topology.Services[serviceKey]
	for key, unit := range svc.Units {
		if unit.Sequence == sequenceNo {
			return key, nil
		}
	}
	return "", fmt.Errorf("unit with sequence number %d not found", sequenceNo)
}

// unitNotAssigned indicates that a unit is not assigned to a machine.
var unitNotAssigned = errors.New("unit not assigned to machine")

// UnitMachineKey returns the key of an assigned machine of the unit. If no machine
// is assigned the error unitNotAssigned will be returned.
func (t *topology) UnitMachineKey(serviceKey, unitKey string) (string, error) {
	if err := t.assertUnit(serviceKey, unitKey); err != nil {
		return "", err
	}
	unit := t.topology.Services[serviceKey].Units[unitKey]
	if unit.Machine == "" {
		return "", unitNotAssigned
	}
	return unit.Machine, nil
}

// AssignUnitToMachine assigns a unit to a machine. It is an error to reassign a 
// unit that is already assigned
func (t *topology) AssignUnitToMachine(serviceKey, unitKey, machineKey string) error {
	err := t.assertUnit(serviceKey, unitKey)
	if err != nil {
		return err
	}
	err = t.assertMachine(machineKey)
	if err != nil {
		return err
	}
	unit := t.topology.Services[serviceKey].Units[unitKey]
	if unit.Machine != "" {
		return fmt.Errorf("unit %q in service %q already assigned to machine %q",
			unitKey, serviceKey, unit.Machine)
	}
	unit.Machine = machineKey
	return nil
}

// UnassignUnitFromMachine unassigns the unit from its current machine.
func (t *topology) UnassignUnitFromMachine(serviceKey, unitKey string) error {
	if err := t.assertUnit(serviceKey, unitKey); err != nil {
		return err
	}
	unit := t.topology.Services[serviceKey].Units[unitKey]
	if unit.Machine == "" {
		return fmt.Errorf("unit %q in service %q not assigned to a machine", unitKey, serviceKey)
	}
	unit.Machine = ""
	return nil
}

// HasRelation returns true of a relation with the given key exists.
func (t *topology) HasRelation(key string) bool {
	return t.topology.Relations[key] != nil
}

// AddClientServerRelation adds a relation between the client and
// the server. It will get the given key, interface and scope.
func (t *topology) AddClientServerRelation(relationKey, clientKey, serverKey, ifce string, scope RelationScope) error {
	if t.topology.Relations == nil {
		t.topology.Relations = make(map[string]*zkRelation)
	}
	if t.HasRelation(relationKey) {
		return fmt.Errorf("relation key %q already in use", relationKey)
	}
	if clientKey == serverKey {
		return fmt.Errorf("client and server keys must not be the same")
	}
	if err := t.assertService(clientKey); err != nil {
		return err
	}
	if err := t.assertService(serverKey); err != nil {
		return err
	}
	t.topology.Relations[relationKey] = &zkRelation{
		Interface: ifce,
		Scope:     scope,
		Members: map[RelationRole]string{
			RoleClient: clientKey,
			RoleServer: serverKey,
		},
	}
	return nil
}

// AddPeerRelation adds a relation with the peer. It
// will get the given key, interface and scope. 
func (t *topology) AddPeerRelation(relationKey, peerKey, ifce string, scope RelationScope) error {
	if t.topology.Relations == nil {
		t.topology.Relations = make(map[string]*zkRelation)
	}
	if t.HasRelation(relationKey) {
		return fmt.Errorf("relation key %q already in use", relationKey)
	}
	if err := t.assertService(peerKey); err != nil {
		return err
	}
	t.topology.Relations[relationKey] = &zkRelation{
		Interface: ifce,
		Scope:     scope,
		Members: map[RelationRole]string{
			RolePeer: peerKey,
		},
	}
	return nil
}

// RelationKeys returns all relation keys.
func (t *topology) RelationKeys() []string {
	keys := []string{}
	for key, _ := range t.topology.Relations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// RelationServices returns the keys and roles of all services which are members 
// of the relation.
func (t *topology) RelationServices(key string) (map[string]RelationRole, error) {
	if err := t.assertRelation(key); err != nil {
		return nil, err
	}
	services := make(map[string]RelationRole)
	for role, serviceKey := range t.topology.Relations[key].Members {
		services[serviceKey] = role
	}
	if len(services) == 0 {
		return nil, nil
	}
	return services, nil
}

// RelationInterface returns the interface of a relation.
func (t *topology) RelationInterface(key string) (string, error) {
	if err := t.assertRelation(key); err != nil {
		return "", err
	}
	return t.topology.Relations[key].Interface, nil
}

// RelationScope returns the scope of a relation.
func (t *topology) RelationScope(key string) (RelationScope, error) {
	if err := t.assertRelation(key); err != nil {
		return ScopeNone, err
	}
	return t.topology.Relations[key].Scope, nil
}

// RelationHasService returns true if 'serviceKey' is 
// a member of the relation.
func (t *topology) RelationHasService(relationKey, serviceKey string) bool {
	if t.HasRelation(relationKey) {
		for _, key := range t.topology.Relations[relationKey].Members {
			if key == serviceKey {
				return true
			}
		}
	}
	return false
}

// RemoveRelation removes a relation.
func (t *topology) RemoveRelation(key string) {
	delete(t.topology.Relations, key)
}

// endpointInfo bundles the information of an endpoint
// between a service and a relation.
type endpointInfo struct {
	ServiceKey  string
	RelationKey string
	Interface   string
	Scope       RelationScope
	Role        RelationRole
}

// ActiveServiceEndpoint returns information about the endpoint
// between a service and a relation.
func (t *topology) ActiveServiceEndpoint(serviceKey, relationKey string) (*endpointInfo, error) {
	if err := t.assertService(serviceKey); err != nil {
		return nil, err
	}
	if err := t.assertRelation(relationKey); err != nil {
		return nil, err
	}
	relation := t.topology.Relations[relationKey]
	for role, key := range relation.Members {
		if key == serviceKey {
			return &endpointInfo{
				ServiceKey:  serviceKey,
				RelationKey: relationKey,
				Interface:   relation.Interface,
				Scope:       relation.Scope,
				Role:        role,
			}, nil
		}
	}
	return nil, fmt.Errorf("service %q is not assigned to relation %q", serviceKey, relationKey)
}

// ActiveServiceEndpoints returns all information of the endpoints
// between a service and its relations.
func (t *topology) ActiveServiceEndpoints(serviceKey string) ([]*endpointInfo, error) {
	if err := t.assertService(serviceKey); err != nil {
		return nil, err
	}
	endpointInfos := []*endpointInfo{}
	for relationKey, relation := range t.topology.Relations {
		for role, key := range relation.Members {
			if key == serviceKey {
				info := &endpointInfo{
					ServiceKey:  serviceKey,
					RelationKey: relationKey,
					Interface:   relation.Interface,
					Scope:       relation.Scope,
					Role:        role,
				}
				endpointInfos = append(endpointInfos, info)
			}
		}
	}
	return endpointInfos, nil
}

// assertMachine checks if a machine exists.
func (t *topology) assertMachine(machineKey string) error {
	if _, ok := t.topology.Machines[machineKey]; !ok {
		return fmt.Errorf("machine with key %q not found", machineKey)
	}
	return nil
}

// assertService checks if a service exists.
func (t *topology) assertService(serviceKey string) error {
	if _, ok := t.topology.Services[serviceKey]; !ok {
		return fmt.Errorf("service with key %q not found", serviceKey)
	}
	return nil
}

// assertUnit checks if a service with a unit exists.
func (t *topology) assertUnit(serviceKey, unitKey string) error {
	if err := t.assertService(serviceKey); err != nil {
		return err
	}
	svc := t.topology.Services[serviceKey]
	if _, ok := svc.Units[unitKey]; !ok {
		return fmt.Errorf("unit with key %q not found", unitKey)
	}
	return nil
}

// assertRelation checks if a relation exists.
func (t *topology) assertRelation(relationKey string) error {
	if _, ok := t.topology.Relations[relationKey]; !ok {
		return fmt.Errorf("relation with key %q not found", relationKey)
	}
	return nil
}

// parseTopology returns the topology represented by yaml.
func parseTopology(yaml string) (*topology, error) {
	t := &topology{topology: &zkTopology{Version: topologyVersion}}
	if err := goyaml.Unmarshal([]byte(yaml), t.topology); err != nil {
		return nil, err
	}
	if t.topology.Version != topologyVersion {
		return nil, fmt.Errorf("incompatible topology versions: got %d, want %d",
			t.topology.Version, topologyVersion)
	}
	return t, nil
}

// retryTopologyChange tries to change the topology with f.
// This function can read and modify the topology instance, 
// and after it returns the modified topology will be
// persisted into the /topology node. Note that this f must
// have no side-effects, since it may be called multiple times
// depending on conflict situations.
func retryTopologyChange(zk *zookeeper.Conn, f func(t *topology) error) error {
	change := func(yaml string, stat *zookeeper.Stat) (string, error) {
		var err error
		it := &topology{topology: &zkTopology{Version: 1}}
		if yaml != "" {
			if it, err = parseTopology(yaml); err != nil {
				return "", err
			}
		}
		// Apply the passed function.
		if err = f(it); err != nil {
			return "", err
		}
		return it.dump()
	}
	return zk.RetryChange("/topology", 0, zkPermAll, change)
}
