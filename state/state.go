// The state package enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net/url"
	"regexp"
)

// TODO(niemeyer): This must not be exported.
type D []bson.DocElem

// Tools describes a particular set of juju tools and where to find them.
type Tools struct {
	version.Binary
	URL string
}

type toolsDoc struct {
	Version version.Binary
	URL     string
}

func (t *Tools) GetBSON() (interface{}, error) {
	if t == nil {
		return nil, nil
	}
	return &toolsDoc{t.Binary, t.URL}, nil
}

func (t *Tools) SetBSON(raw bson.Raw) error {
	if raw.Kind == 10 {
		// Preserve the nil value in that case.
		return bson.SetZero
	}
	var doc toolsDoc
	if err := raw.Unmarshal(&doc); err != nil {
		return err
	}
	t.Binary = doc.Version
	t.URL = doc.URL
	return nil
}

var (
	validService = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
	validUnit    = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*/[0-9]+$")
)

// IsServiceName returns whether name is a valid service name.
func IsServiceName(name string) bool {
	return validService.MatchString(name)
}

// IsUnitName returns whether name is a valid unit name.
func IsUnitName(name string) bool {
	return validUnit.MatchString(name)
}

// NotFoundError represents the error that something is not found.
type NotFoundError struct {
	msg string
}

func (e *NotFoundError) Error() string {
	return e.msg
}

func notFound(format string, args ...interface{}) error {
	return &NotFoundError{fmt.Sprintf(format+" not found", args...)}
}

func IsNotFound(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}

// State represents the state of an environment
// managed by juju.
type State struct {
	db             *mgo.Database
	charms         *mgo.Collection
	machines       *mgo.Collection
	relations      *mgo.Collection
	relationScopes *mgo.Collection
	services       *mgo.Collection
	settings       *mgo.Collection
	units          *mgo.Collection
	presence       *mgo.Collection
	runner         *txn.Runner
	watcher        *watcher.Watcher
	pwatcher       *presence.Watcher
	fwd            *sshForwarder
}

func (s *State) EnvironConfig() (*config.Config, error) {
	configNode, err := readConfigNode(s, "e")
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
	_, err := createConfigNode(s, "e", attrs)
	return err
}

type WorkerKind string

const (
	MachinerWorker    WorkerKind = "machiner"
	ProvisionerWorker WorkerKind = "provisioner"
	FirewallerWorker  WorkerKind = "firewaller"
)

// AddMachine adds a new machine that when deployed will have a
// machine agent running the provided workers.
func (s *State) AddMachine(workers ...WorkerKind) (m *Machine, err error) {
	defer trivial.ErrorContextf(&err, "cannot add a new machine")
	wset := make(map[WorkerKind]bool)
	for _, w := range workers {
		if wset[w] {
			return nil, fmt.Errorf("duplicate worker: %s", w)
		}
		wset[w] = true
	}
	if !wset[MachinerWorker] {
		return nil, fmt.Errorf("new machine must be started with a machine worker")
	}
	id, err := s.sequence("machine")
	if err != nil {
		return nil, err
	}
	mdoc := machineDoc{
		Id:      id,
		Life:    Alive,
		Workers: workers,
	}
	ops := []txn.Op{{
		C:      s.machines.Name,
		Id:     id,
		Assert: txn.DocMissing,
		Insert: mdoc,
	}}
	err = s.runner.Run(ops, "", nil)
	if err != nil {
		return nil, err
	}
	// Refresh to pick the txn-revno.
	m = newMachine(s, &mdoc)
	if err = m.Refresh(); err != nil {
		return nil, err
	}
	return m, nil
}

var errNotAlive = fmt.Errorf("not found or not alive")

func onAbort(txnErr, err error) error {
	if txnErr == txn.ErrAborted {
		return err
	}
	return txnErr
}

// RemoveMachine removes the machine with the the given id.
func (s *State) RemoveMachine(id int) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove machine %d", id)
	m, err := s.Machine(id)
	if err != nil {
		return err
	}
	if m.doc.Life != Dead {
		return fmt.Errorf("machine is not dead")
	}
	sel := D{
		{"_id", id},
		{"life", Dead},
	}
	ops := []txn.Op{{
		C:      s.machines.Name,
		Id:     id,
		Assert: sel,
		Remove: true,
	}}
	if err := s.runner.Run(ops, "", nil); err != nil {
		// If aborted, the machine is either dead or recreated.
		return onAbort(err, nil)
	}
	return nil
}

// AllMachines returns all machines in the environment
// ordered by id.
func (s *State) AllMachines() (machines []*Machine, err error) {
	mdocs := []machineDoc{}
	err = s.machines.Find(nil).Sort("_id").All(&mdocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all machines: %v", err)
	}
	for _, doc := range mdocs {
		machines = append(machines, newMachine(s, &doc))
	}
	return
}

// Machine returns the machine with the given id.
func (s *State) Machine(id int) (*Machine, error) {
	mdoc := &machineDoc{}
	sel := D{{"_id", id}}
	err := s.machines.Find(sel).One(mdoc)
	if err == mgo.ErrNotFound {
		return nil, notFound("machine %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get machine %d: %v", id, err)
	}
	return newMachine(s, mdoc), nil
}

// AddCharm adds the ch charm with curl to the state.  bundleUrl must be
// set to a URL where the bundle for ch may be downloaded from.
// On success the newly added charm state is returned.
func (s *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (stch *Charm, err error) {
	cdoc := &charmDoc{
		URL:          curl,
		Meta:         ch.Meta(),
		Config:       ch.Config(),
		BundleURL:    bundleURL,
		BundleSha256: bundleSha256,
	}
	err = s.charms.Insert(cdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm %q: %v", curl, err)
	}
	return newCharm(s, cdoc)
}

// Charm returns the charm with the given URL.
func (s *State) Charm(curl *charm.URL) (*Charm, error) {
	cdoc := &charmDoc{}
	err := s.charms.Find(D{{"_id", curl}}).One(cdoc)
	if err == mgo.ErrNotFound {
		return nil, notFound("charm %q", curl)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get charm %q: %v", curl, err)
	}
	return newCharm(s, cdoc)
}

// AddService creates a new service state with the given unique name
// and the charm state.
func (s *State) AddService(name string, ch *Charm) (service *Service, err error) {
	if !IsServiceName(name) {
		return nil, fmt.Errorf("%q is not a valid service name", name)
	}
	sdoc := &serviceDoc{
		Name:     name,
		CharmURL: ch.URL(),
		Life:     Alive,
	}
	ops := []txn.Op{{
		C:      s.services.Name,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: sdoc,
	}}
	if err := s.runner.Run(ops, "", nil); err != nil {
		return nil, fmt.Errorf("cannot add service %q: %v", name, onAbort(err, fmt.Errorf("duplicate service name")))
	}
	// Refresh to pick the txn-revno.
	svc := newService(s, sdoc)
	if err = svc.Refresh(); err != nil {
		return nil, err
	}
	return svc, nil
}

func (s *State) setPassword(pathKey, password string) (auth string, err error) {
	if err := s.db.AddUser(pathKey, password, false); err != nil {
		return "", fmt.Errorf("cannot set password for %q in db: %v", name, err)
	}
	if err := s.presence.AddUser(pathKey, password, false); err != nil {
		return "", fmt.Errorf("cannot set password for %q in presence: %v", name, err)
	}
	return name + "/" + password, nil
}

// SetPassword sets the password the administrator
// should use to communicate with the state servers.  Previous passwords
// are invalidated. The returned authorization can be used in
// the Auth field of the Info value when calling Open.
func (s *State) SetPassword(password string) (auth string, err error) {
	return s.setPassword("admin", password)
}

// RemoveService removes a service from the state. It will also remove all
// its units and break any of its existing relations.
func (s *State) RemoveService(svc *Service) (err error) {
	// TODO Do lifecycle properly.
	// Removing relations and units here is wrong. They need to monitor
	// their own parent and set themselves to dying.
	defer trivial.ErrorContextf(&err, "cannot remove service %q", svc)

	if svc.doc.Life != Dead {
		return fmt.Errorf("service is not dead")
	}
	rels, err := svc.Relations()
	if err != nil {
		return err
	}
	for _, rel := range rels {
		err = rel.EnsureDead()
		if err != nil {
			return err
		}
		err = s.RemoveRelation(rel)
		if err != nil {
			return err
		}
	}
	units, err := svc.AllUnits()
	if err != nil {
		return err
	}
	for _, unit := range units {
		err = unit.EnsureDead()
		if err != nil {
			return err
		}
		if err = svc.RemoveUnit(unit); err != nil {
			return err
		}
	}
	ops := []txn.Op{{
		C:      s.services.Name,
		Id:     svc.doc.Name,
		Assert: D{{"life", Dead}},
		Remove: true,
	}}
	if err := s.runner.Run(ops, "", nil); err != nil {
		// If aborted, the service is either dead or recreated.
		return onAbort(err, nil)
	}
	return nil
}

// Service returns a service state by name.
func (s *State) Service(name string) (service *Service, err error) {
	if !IsServiceName(name) {
		return nil, fmt.Errorf("%q is not a valid service name", name)
	}
	sdoc := &serviceDoc{}
	sel := D{{"_id", name}}
	err = s.services.Find(sel).One(sdoc)
	if err == mgo.ErrNotFound {
		return nil, notFound("service %q", name)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get service %q: %v", name, err)
	}
	return newService(s, sdoc), nil
}

// AllServices returns all deployed services in the environment.
func (s *State) AllServices() (services []*Service, err error) {
	sdocs := []serviceDoc{}
	err = s.services.Find(D{}).All(&sdocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all services")
	}
	for _, v := range sdocs {
		services = append(services, newService(s, &v))
	}
	return services, nil
}

// AddRelation creates a new relation with the given endpoints.
func (s *State) AddRelation(endpoints ...RelationEndpoint) (r *Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot add relation %q", relationKey(endpoints))
	switch len(endpoints) {
	case 1:
		if endpoints[0].RelationRole != RolePeer {
			return nil, fmt.Errorf("single endpoint must be a peer relation")
		}
	case 2:
		if !endpoints[0].CanRelateTo(&endpoints[1]) {
			return nil, fmt.Errorf("endpoints do not relate")
		}
	default:
		return nil, fmt.Errorf("cannot relate %d endpoints", len(endpoints))
	}

	var scope charm.RelationScope
	for _, v := range endpoints {
		if v.RelationScope == charm.ScopeContainer {
			scope = charm.ScopeContainer
		}
		// BUG(aram): potential race in the time between getting the service
		// to validate the endpoint and actually writting the relation
		// into MongoDB; the service might have disappeared.
		_, err = s.Service(v.ServiceName)
		if err != nil {
			return nil, err
		}
	}
	if scope == charm.ScopeContainer {
		for i := range endpoints {
			endpoints[i].RelationScope = scope
		}
	}
	id, err := s.sequence("relation")
	if err != nil {
		return nil, err
	}
	doc := relationDoc{
		Key:       relationKey(endpoints),
		Id:        id,
		Endpoints: endpoints,
		Life:      Alive,
	}
	ops := []txn.Op{{
		C:      s.relations.Name,
		Id:     doc.Key,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	err = s.runner.Run(ops, "", nil)
	if err != nil {
		return nil, err
	}
	return newRelation(s, &doc), nil
}

// Relation returns the existing relation with the given endpoints.
func (s *State) Relation(endpoints ...RelationEndpoint) (*Relation, error) {
	doc := relationDoc{}
	key := relationKey(endpoints)
	err := s.relations.Find(D{{"_id", key}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, notFound("relation %q", key)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get relation %q: %v", key, err)
	}
	return newRelation(s, &doc), nil
}

// RemoveRelation removes the supplied relation.
func (s *State) RemoveRelation(r *Relation) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove relation %q", r.doc.Key)
	if r.doc.Life != Dead {
		return fmt.Errorf("relation is not dead")
	}
	ops := []txn.Op{{
		C:      s.relations.Name,
		Id:     r.doc.Key,
		Assert: D{{"life", Dead}},
		Remove: true,
	}}
	if err := s.runner.Run(ops, "", nil); err != nil {
		// If aborted, the relation is either dead or recreated.
		return onAbort(err, nil)
	}
	return nil
}

// Unit returns a unit by name.
func (s *State) Unit(name string) (*Unit, error) {
	if !IsUnitName(name) {
		return nil, fmt.Errorf("%q is not a valid unit name", name)
	}
	doc := unitDoc{}
	err := s.units.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, notFound("unit %q", name)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get unit %q: %v", name, err)
	}
	return newUnit(s, &doc), nil
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
		m, err = s.Machine(0)
		if err != nil {
			return err
		}
		return u.AssignToMachine(m)
	case AssignUnused:
		if _, err = u.AssignToUnusedMachine(); err != noUnusedMachines {
			return err
		}
		m, err := s.AddMachine(MachinerWorker)
		if err != nil {
			return err
		}
		return u.AssignToMachine(m)

		// TODO(rog) reinstate this code
		// This works if two AssignUnits are racing each other,
		// but might not if someone picks the machine we've
		// just created and tries to assign a unit to that machine
		// specifically. This should never happen in practice.
		_, err = u.AssignToUnusedMachine()
		return err
	}
	panic(fmt.Errorf("unknown unit assignment policy: %q", policy))
}

// StartSync forces watchers to resynchronize their state with the
// database immediately. This will happen periodically automatically.
func (s *State) StartSync() {
	s.watcher.StartSync()
	s.pwatcher.StartSync()
}

// Sync forces watchers to resynchronize their state with the
// database immediately, and waits until all events are known.
func (s *State) Sync() {
	s.watcher.Sync()
	s.pwatcher.Sync()
}
