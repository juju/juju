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
	"launchpad.net/juju-core/log"
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
	cleanups       *mgo.Collection
	runner         *txn.Runner
	watcher        *watcher.Watcher
	pwatcher       *presence.Watcher
	fwd            *sshForwarder
}

func (s *State) EnvironConfig() (*config.Config, error) {
	settings, err := readSettings(s, "e")
	if err != nil {
		return nil, err
	}
	attrs := settings.Map()
	return config.New(attrs)
}

// SetEnvironConfig replaces the current configuration of the 
// environment with the provided configuration.
func (s *State) SetEnvironConfig(cfg *config.Config) error {
	if cfg.AdminSecret() != "" {
		return fmt.Errorf("admin-secret should never be written to the state")
	}
	// TODO(niemeyer): This isn't entirely right as the change is done as a
	// delta that the user didn't ask for. Instead, take a (old, new) config
	// pair, and apply *known* delta.
	settings, err := readSettings(s, "e")
	if err != nil {
		return err
	}
	settings.Update(cfg.AllAttrs())
	_, err = settings.Write()
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
	svc := newService(s, sdoc)
	ops := []txn.Op{{
		C:      s.settings.Name,
		Id:     svc.globalKey(),
		Insert: D{},
	}, {
		C:      s.services.Name,
		Id:     name,
		Assert: txn.DocMissing,
		Insert: sdoc,
	}}
	if err := s.runner.Run(ops, "", nil); err != nil {
		return nil, fmt.Errorf("cannot add service %q: %v", name, onAbort(err, fmt.Errorf("duplicate service name")))
	}
	// Refresh to pick the txn-revno.
	if err = svc.Refresh(); err != nil {
		return nil, err
	}
	return svc, nil
}

// RemoveService removes a service from the state.
func (s *State) RemoveService(svc *Service) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove service %q", svc)
	if svc.doc.Life != Dead {
		return fmt.Errorf("service is not dead")
	}
	ops := []txn.Op{{
		C:      s.services.Name,
		Id:     svc.doc.Name,
		Assert: D{{"life", Dead}},
		Remove: true,
	}, {
		C:      s.settings.Name,
		Id:     svc.globalKey(),
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
func (s *State) AddRelation(endpoints ...Endpoint) (r *Relation, err error) {
	defer trivial.ErrorContextf(&err, "cannot add relation %q", relationKey(endpoints))
	switch len(endpoints) {
	case 1:
		if endpoints[0].RelationRole != RolePeer {
			return nil, fmt.Errorf("single endpoint must be a peer relation")
		}
	case 2:
		if !endpoints[0].CanRelateTo(endpoints[1]) {
			return nil, fmt.Errorf("endpoints do not relate")
		}
	default:
		return nil, fmt.Errorf("cannot relate %d endpoints", len(endpoints))
	}

	ops := []txn.Op{}
	var scope charm.RelationScope
	for _, v := range endpoints {
		if v.RelationScope == charm.ScopeContainer {
			scope = charm.ScopeContainer
		}
		ops = append(ops, txn.Op{
			C:      s.services.Name,
			Id:     v.ServiceName,
			Assert: isAlive,
			Update: D{{"$inc", D{{"relationcount", 1}}}},
		})
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
	ops = append(ops, txn.Op{
		C:      s.relations.Name,
		Id:     doc.Key,
		Assert: txn.DocMissing,
		Insert: doc,
	})
	err = s.runner.Run(ops, "", nil)
	if err == txn.ErrAborted {
		for _, ep := range endpoints {
			svc, err := s.Service(ep.ServiceName)
			if IsNotFound(err) || svc.Life() != Alive {
				return nil, fmt.Errorf("service %q is not alive", ep.ServiceName)
			} else if err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("relation already exists")
	} else if err != nil {
		return nil, err
	}
	return newRelation(s, &doc), nil
}

// EndpointsRelation returns the existing relation with the given endpoints.
func (s *State) EndpointsRelation(endpoints ...Endpoint) (*Relation, error) {
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

// Relation returns the existing relation with the given id.
func (s *State) Relation(id int) (*Relation, error) {
	doc := relationDoc{}
	err := s.relations.Find(D{{"id", id}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, notFound("relation %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get relation %d: %v", id, err)
	}
	return newRelation(s, &doc), nil
}

// RemoveRelation removes the supplied relation and all its unit settings.
func (s *State) RemoveRelation(r *Relation) (err error) {
	defer trivial.ErrorContextf(&err, "cannot remove relation %q", r.doc.Key)
	if r.doc.Life != Dead {
		return fmt.Errorf("relation is not dead")
	}
	if err := s.runner.Run(r.removeOps(D{{"life", Dead}}), "", nil); err != nil {
		if err == txn.ErrAborted {
			if e := r.Refresh(); IsNotFound(e) {
				return nil
			} else if e != nil {
				return e
			}
			return fmt.Errorf("cannot remove relation %q: inconsistent state", r)
		}
		return err
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
		for {
			// TODO(rog) take out a lease on the new machine
			// so that we don't have a race here.
			m, err := s.AddMachine(MachinerWorker)
			if err != nil {
				return err
			}
			err = u.assignToMachine(m, true)
			if err == inUseErr {
				// Someone else has grabbed the machine we've
				// just allocated, so try again.
				continue
			}
			return err
		}
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

// SetAdminPassword sets the administrative password
// to access the state. If the password is non-empty,
// all subsequent attempts to access the state must
// be authorized; otherwise no authorization is required.
func (s *State) SetAdminPassword(password string) error {
	admin := s.db.Session.DB("admin")
	if password != "" {
		// On 2.2+, we get a "need to login" error without a code when
		// adding the first user because we go from no-auth+no-login to
		// auth+no-login. Not great. Hopefully being fixed in 2.4.
		if err := admin.AddUser("admin", password, false); err != nil && err.Error() != "need to login" {
			return fmt.Errorf("cannot set admin password: %v", err)
		}
		if err := admin.Login("admin", password); err != nil {
			return fmt.Errorf("cannot login after setting password: %v", err)
		}
	} else {
		if err := admin.RemoveUser("admin"); err != nil && err != mgo.ErrNotFound {
			return fmt.Errorf("cannot disable admin password: %v", err)
		}
	}
	return nil
}

func (s *State) setPassword(name, password string) error {
	if err := s.db.AddUser(name, password, false); err != nil {
		return fmt.Errorf("cannot set password in juju db for %q: %v", name, err)
	}
	if err := s.db.Session.DB("presence").AddUser(name, password, false); err != nil {
		return fmt.Errorf("cannot set password in presence db for %q: %v", name, err)
	}
	return nil
}

// cleanupDoc represents a potentially large set of documents that should be
// removed.
type cleanupDoc struct {
	Id     bson.ObjectId `bson:"_id"`
	Kind   string
	Prefix string
}

// Cleanup removes all documents that were previously marked for removal, if
// any such exist. It should be called periodically by at least one element
// of the system.
func (s *State) Cleanup() error {
	doc := cleanupDoc{}
	iter := s.cleanups.Find(nil).Iter()
	for iter.Next(&doc) {
		var c *mgo.Collection
		var sel interface{}
		switch doc.Kind {
		case "settings":
			c = s.settings
			sel = D{{"_id", D{{"$regex", "^" + doc.Prefix}}}}
		default:
			log.Printf("state: WARNING: ignoring unknown cleanup kind %q", doc.Kind)
			continue
		}
		if count, err := c.Find(sel).Count(); err != nil {
			return fmt.Errorf("cannot detect cleanup targets: %v", err)
		} else if count != 0 {
			// Documents marked for cleanup are not otherwise referenced in the
			// system, and will not be under watch, and are therefore safe to
			// delete directly.
			if _, err := c.RemoveAll(sel); err != nil {
				return fmt.Errorf("cannot remove documents marked for cleanup: %v", err)
			}
		}
		ops := []txn.Op{{
			C:      s.cleanups.Name,
			Id:     doc.Id,
			Remove: true,
		}}
		if err := s.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot remove empty cleanup document: %v", err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read cleanup document: %v", err)
	}
	return nil
}
