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
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/trivial"
	"launchpad.net/juju-core/version"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

const serviceSnippet = "[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*"

var (
	validService = regexp.MustCompile("^" + serviceSnippet + "$")
	validUnit    = regexp.MustCompile("^" + serviceSnippet + "(-[a-z0-9]*[a-z][a-z0-9]*)*/[0-9]+$")
	validMachine = regexp.MustCompile("^0$|^[1-9][0-9]*$")
)

// IsServiceName returns whether name is a valid service name.
func IsServiceName(name string) bool {
	return validService.MatchString(name)
}

// IsUnitName returns whether name is a valid unit name.
func IsUnitName(name string) bool {
	return validUnit.MatchString(name)
}

// IsMachineId returns whether id is a valid machine id.
func IsMachineId(name string) bool {
	return validMachine.MatchString(name)
}

// notFoundError represents the error that something is not found.
type notFoundError struct {
	msg string
}

func (e *notFoundError) Error() string {
	return e.msg
}

// NotFoundf returns a error for which IsNotFound returns
// true. The message for the error is made up from the given
// arguments formatted as with fmt.Sprintf, with the
// string " not found" appended.
func NotFoundf(format string, args ...interface{}) error {
	return &notFoundError{fmt.Sprintf(format+" not found", args...)}
}

func IsNotFound(err error) bool {
	_, ok := err.(*notFoundError)
	return ok
}

// State represents the state of an environment
// managed by juju.
type State struct {
	info           *Info
	db             *mgo.Database
	charms         *mgo.Collection
	machines       *mgo.Collection
	relations      *mgo.Collection
	relationScopes *mgo.Collection
	services       *mgo.Collection
	settings       *mgo.Collection
	settingsrefs   *mgo.Collection
	constraints    *mgo.Collection
	units          *mgo.Collection
	users          *mgo.Collection
	presence       *mgo.Collection
	cleanups       *mgo.Collection
	annotations    *mgo.Collection
	runner         *txn.Runner
	watcher        *watcher.Watcher
	pwatcher       *presence.Watcher
	allWatcher     *allWatcher
}

func (st *State) Watch() *StateWatcher {
	return newStateWatcher(st)
}

func (st *State) EnvironConfig() (*config.Config, error) {
	settings, err := readSettings(st, "e")
	if err != nil {
		return nil, err
	}
	attrs := settings.Map()
	return config.New(attrs)
}

// SetEnvironConfig replaces the current configuration of the
// environment with the provided configuration.
func (st *State) SetEnvironConfig(cfg *config.Config) error {
	if cfg.AdminSecret() != "" {
		return fmt.Errorf("admin-secret should never be written to the state")
	}
	// TODO(niemeyer): This isn't entirely right as the change is done as a
	// delta that the user didn't ask for. Instead, take a (old, new) config
	// pair, and apply *known* delta.
	settings, err := readSettings(st, "e")
	if err != nil {
		return err
	}
	settings.Update(cfg.AllAttrs())
	_, err = settings.Write()
	return err
}

// EnvironConstraints returns the current environment constraints.
func (st *State) EnvironConstraints() (constraints.Value, error) {
	return readConstraints(st, "e")
}

// SetEnvironConstraints replaces the current environment constraints.
func (st *State) SetEnvironConstraints(cons constraints.Value) error {
	return writeConstraints(st, "e", cons)
}

// AddMachine adds a new machine configured to run the supplied jobs on the
// supplied series.
func (st *State) AddMachine(series string, jobs ...MachineJob) (m *Machine, err error) {
	return st.addMachine(series, "", jobs)
}

// InjectMachine adds a new machine, corresponding to an existing provider
// instance, configured to run the supplied jobs on the supplied series.
func (st *State) InjectMachine(series string, instanceId InstanceId, jobs ...MachineJob) (m *Machine, err error) {
	if instanceId == "" {
		return nil, fmt.Errorf("cannot inject a machine without an instance id")
	}
	return st.addMachine(series, instanceId, jobs)
}

func (st *State) addMachineOps(mdoc *machineDoc) (*machineDoc, []txn.Op, error) {
	if mdoc.Series == "" {
		return nil, nil, fmt.Errorf("no series specified")
	}
	if len(mdoc.Jobs) == 0 {
		return nil, nil, fmt.Errorf("no jobs specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range mdoc.Jobs {
		if jset[j] {
			return nil, nil, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	seq, err := st.sequence("machine")
	if err != nil {
		return nil, nil, err
	}
	mdoc.Id = strconv.Itoa(seq)
	mdoc.Life = Alive
	ops := []txn.Op{{
		C:      st.machines.Name,
		Id:     mdoc.Id,
		Assert: txn.DocMissing,
		Insert: *mdoc,
	}}
	return mdoc, ops, nil
}

// addMachine implements AddMachine and InjectMachine.
func (st *State) addMachine(series string, instanceId InstanceId, jobs []MachineJob) (m *Machine, err error) {
	defer trivial.ErrorContextf(&err, "cannot add a new machine")

	mdoc := &machineDoc{
		Series:     series,
		InstanceId: instanceId,
		Jobs:       jobs,
	}
	mdoc, ops, err := st.addMachineOps(mdoc)
	if err != nil {
		return nil, err
	}

	err = st.runner.Run(ops, "", nil)
	if err != nil {
		return nil, err
	}
	// Refresh to pick the txn-revno.
	m = newMachine(st, mdoc)
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

// AllMachines returns all machines in the environment
// ordered by id.
func (st *State) AllMachines() (machines []*Machine, err error) {
	mdocs := machineDocSlice{}
	err = st.machines.Find(nil).All(&mdocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all machines: %v", err)
	}
	sort.Sort(mdocs)
	for _, doc := range mdocs {
		machines = append(machines, newMachine(st, &doc))
	}
	return
}

type machineDocSlice []machineDoc

func (ms machineDocSlice) Len() int      { return len(ms) }
func (ms machineDocSlice) Swap(i, j int) { ms[i], ms[j] = ms[j], ms[i] }
func (ms machineDocSlice) Less(i, j int) bool {
	// There's nothing we can do with errors at this point.
	m1, _ := strconv.Atoi(ms[i].Id)
	m2, _ := strconv.Atoi(ms[j].Id)
	return m1 < m2
}

// Machine returns the machine with the given id.
func (st *State) Machine(id string) (*Machine, error) {
	mdoc := &machineDoc{}
	sel := D{{"_id", id}}
	err := st.machines.Find(sel).One(mdoc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("machine %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get machine %s: %v", id, err)
	}
	return newMachine(st, mdoc), nil
}

// Authenticator represents entites capable of handling password
// authentication.
type Authenticator interface {
	Refresh() error
	SetPassword(pass string) error
	PasswordValid(pass string) bool
	EntityName() string
}

// Annotator represents entities capable of handling annotations.
type Annotator interface {
	Annotation(key string) (string, error)
	Annotations() (map[string]string, error)
	SetAnnotations(pairs map[string]string) error
}

// Authenticator attempts to return an Authenticator with the given name.
func (st *State) Authenticator(name string) (Authenticator, error) {
	e, err := st.entity(name)
	if err != nil {
		return nil, err
	}
	if e, ok := e.(Authenticator); ok {
		return e, nil
	}
	return nil, fmt.Errorf("entity %q does not support authentication", name)
}

// Annotator attempts to return an Annotator with the given name.
func (st *State) Annotator(name string) (Annotator, error) {
	e, err := st.entity(name)
	if err != nil {
		return nil, err
	}
	if e, ok := e.(Annotator); ok {
		return e, nil
	}
	return nil, fmt.Errorf("entity %q does not support annotations", name)
}

// entity returns the entity for the given name.
func (st *State) entity(entityName string) (interface{}, error) {
	i := strings.Index(entityName, "-")
	if i <= 0 || i >= len(entityName)-1 {
		return nil, fmt.Errorf("invalid entity name %q", entityName)
	}
	prefix, id := entityName[0:i], entityName[i+1:]
	switch prefix {
	case "machine":
		if !IsMachineId(id) {
			return nil, fmt.Errorf("invalid entity name %q", entityName)
		}
		return st.Machine(id)
	case "unit":
		i := strings.LastIndex(id, "-")
		if i == -1 {
			return nil, fmt.Errorf("invalid entity name %q", entityName)
		}
		name := id[:i] + "/" + id[i+1:]
		if !IsUnitName(name) {
			return nil, fmt.Errorf("invalid entity name %q", entityName)
		}
		return st.Unit(name)
	case "user":
		return st.User(id)
	case "service":
		if !IsServiceName(id) {
			return nil, fmt.Errorf("invalid entity name %q", entityName)
		}
		return st.Service(id)
	case "environment":
		conf, err := st.EnvironConfig()
		if err != nil {
			return nil, err
		}
		// Return an invalid entity error if the requested environment is not
		// the current one.
		if id != conf.Name() {
			return nil, fmt.Errorf("invalid entity name %q", entityName)
		}
		return st.Environment()
	}
	return nil, fmt.Errorf("invalid entity name %q", entityName)
}

// ParseEntityName, given an entity name, returns the collection name and id
// of the entity document.
func (st *State) ParseEntityName(entityName string) (string, string, error) {
	parts := strings.SplitN(entityName, "-", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid entity name %q", entityName)
	}
	id := parts[1]
	var coll string
	switch parts[0] {
	case "machine":
		coll = st.machines.Name
	case "service":
		coll = st.services.Name
	case "unit":
		coll = st.units.Name
		// Handle replacements occurring when an entity name is created
		// for a unit.
		idx := strings.LastIndex(id, "-")
		if idx == -1 {
			return "", "", fmt.Errorf("invalid entity name %q", entityName)
		}
		id = id[:idx] + "/" + id[idx+1:]
	case "user":
		coll = st.users.Name
	default:
		return "", "", fmt.Errorf("invalid entity name %q", entityName)
	}
	return coll, id, nil
}

// AddCharm adds the ch charm with curl to the state.  bundleUrl must be
// set to a URL where the bundle for ch may be downloaded from.
// On success the newly added charm state is returned.
func (st *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (stch *Charm, err error) {
	cdoc := &charmDoc{
		URL:          curl,
		Meta:         ch.Meta(),
		Config:       ch.Config(),
		BundleURL:    bundleURL,
		BundleSha256: bundleSha256,
	}
	err = st.charms.Insert(cdoc)
	if err != nil {
		return nil, fmt.Errorf("cannot add charm %q: %v", curl, err)
	}
	return newCharm(st, cdoc)
}

// Charm returns the charm with the given URL.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	cdoc := &charmDoc{}
	err := st.charms.Find(D{{"_id", curl}}).One(cdoc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get charm %q: %v", curl, err)
	}
	if err := cdoc.Meta.Check(); err != nil {
		return nil, fmt.Errorf("malformed charm metadata found in state: %v", err)
	}
	return newCharm(st, cdoc)
}

// addPeerRelationsOps returns the operations necessary to add the
// specified service peer relations to the state.
func (st *State) addPeerRelationsOps(serviceName string, peers map[string]charm.Relation) ([]txn.Op, error) {
	var ops []txn.Op
	for _, rel := range peers {
		relId, err := st.sequence("relation")
		if err != nil {
			return nil, err
		}
		eps := []Endpoint{{
			ServiceName: serviceName,
			Relation:    rel,
		}}
		relKey := relationKey(eps)
		relDoc := &relationDoc{
			Key:       relKey,
			Id:        relId,
			Endpoints: eps,
			Life:      Alive,
		}
		ops = append(ops, txn.Op{
			C:      st.relations.Name,
			Id:     relKey,
			Assert: txn.DocMissing,
			Insert: relDoc,
		})
	}
	return ops, nil
}

// AddService creates a new service, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddService(name string, ch *Charm) (service *Service, err error) {
	defer trivial.ErrorContextf(&err, "cannot add service %q", name)
	// Sanity checks.
	if !IsServiceName(name) {
		return nil, fmt.Errorf("invalid name")
	}
	if ch == nil {
		return nil, fmt.Errorf("charm is nil")
	}
	if exists, err := isNotDead(st.services, name); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("service already exists")
	}
	// Create the service addition operations.
	peers := ch.Meta().Peers
	svcDoc := &serviceDoc{
		Name:          name,
		CharmURL:      ch.URL(),
		RelationCount: len(peers),
		Life:          Alive,
	}
	svc := newService(st, svcDoc)
	ops := []txn.Op{
		createConstraintsOp(st, svc.globalKey(), constraints.Value{}),
		createSettingsOp(st, svc.settingsKey(), nil),
		{
			C:      st.settingsrefs.Name,
			Id:     svc.settingsKey(),
			Assert: txn.DocMissing,
			Insert: settingsRefsDoc{1},
		},
		{
			C:      st.services.Name,
			Id:     name,
			Assert: txn.DocMissing,
			Insert: svcDoc,
		}}
	// Collect peer relation addition operations.
	peerOps, err := st.addPeerRelationsOps(name, peers)
	if err != nil {
		return nil, err
	}
	ops = append(ops, peerOps...)

	// Run the transaction; happily, there's never any reason to retry,
	// because all the possible failed assertions imply that the service
	// already exists.
	if err := st.runner.Run(ops, "", nil); err == txn.ErrAborted {
		return nil, fmt.Errorf("service already exists")
	} else if err != nil {
		return nil, err
	}
	// Refresh to pick the txn-revno.
	if err = svc.Refresh(); err != nil {
		return nil, err
	}
	return svc, nil
}

// Service returns a service state by name.
func (st *State) Service(name string) (service *Service, err error) {
	if !IsServiceName(name) {
		return nil, fmt.Errorf("%q is not a valid service name", name)
	}
	sdoc := &serviceDoc{}
	sel := D{{"_id", name}}
	err = st.services.Find(sel).One(sdoc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("service %q", name)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get service %q: %v", name, err)
	}
	return newService(st, sdoc), nil
}

// AllServices returns all deployed services in the environment.
func (st *State) AllServices() (services []*Service, err error) {
	sdocs := []serviceDoc{}
	err = st.services.Find(D{}).All(&sdocs)
	if err != nil {
		return nil, fmt.Errorf("cannot get all services")
	}
	for _, v := range sdocs {
		services = append(services, newService(st, &v))
	}
	return services, nil
}

// InferEndpoints returns the endpoints corresponding to the supplied names.
// There must be 1 or 2 supplied names, of the form <service>[:<relation>].
// If the supplied names uniquely specify a possible relation, or if they
// uniquely specify a possible relation once all implicit relations have been
// filtered, the endpoints corresponding to that relation will be returned.
func (st *State) InferEndpoints(names []string) ([]Endpoint, error) {
	// Collect all possible sane endpoint lists.
	var candidates [][]Endpoint
	switch len(names) {
	case 1:
		eps, err := st.endpoints(names[0], isPeer)
		if err != nil {
			return nil, err
		}
		for _, ep := range eps {
			candidates = append(candidates, []Endpoint{ep})
		}
	case 2:
		eps1, err := st.endpoints(names[0], notPeer)
		if err != nil {
			return nil, err
		}
		eps2, err := st.endpoints(names[1], notPeer)
		if err != nil {
			return nil, err
		}
		for _, ep1 := range eps1 {
			for _, ep2 := range eps2 {
				if ep1.CanRelateTo(ep2) {
					candidates = append(candidates, []Endpoint{ep1, ep2})
				}
			}
		}
	default:
		return nil, fmt.Errorf("cannot relate %d endpoints", len(names))
	}
	// If there's ambiguity, try discarding implicit relations.
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("no relations found")
	case 1:
		return candidates[0], nil
	}
	var filtered [][]Endpoint
outer:
	for _, cand := range candidates {
		for _, ep := range cand {
			if ep.IsImplicit() {
				continue outer
			}
		}
		filtered = append(filtered, cand)
	}
	if len(filtered) == 1 {
		return filtered[0], nil
	}
	keys := []string{}
	for _, cand := range candidates {
		keys = append(keys, fmt.Sprintf("%q", relationKey(cand)))
	}
	sort.Strings(keys)
	return nil, fmt.Errorf("ambiguous relation: %q could refer to %s",
		strings.Join(names, " "), strings.Join(keys, "; "))
}

func isPeer(ep Endpoint) bool {
	return ep.Role == charm.RolePeer
}

func notPeer(ep Endpoint) bool {
	return ep.Role != charm.RolePeer
}

// endpoints returns all endpoints that could be intended by the
// supplied endpoint name, and which cause the filter param to
// return true.
func (st *State) endpoints(name string, filter func(ep Endpoint) bool) ([]Endpoint, error) {
	var svcName, relName string
	if i := strings.Index(name, ":"); i == -1 {
		svcName = name
	} else if i != 0 && i != len(name)-1 {
		svcName = name[:i]
		relName = name[i+1:]
	} else {
		return nil, fmt.Errorf("invalid endpoint %q", name)
	}
	svc, err := st.Service(svcName)
	if err != nil {
		return nil, err
	}
	eps := []Endpoint{}
	if relName != "" {
		ep, err := svc.Endpoint(relName)
		if err != nil {
			return nil, err
		}
		eps = append(eps, ep)
	} else {
		eps, err = svc.Endpoints()
		if err != nil {
			return nil, err
		}
	}
	final := []Endpoint{}
	for _, ep := range eps {
		if filter(ep) {
			final = append(final, ep)
		}
	}
	return final, nil
}

// AddRelation creates a new relation with the given endpoints.
func (st *State) AddRelation(eps ...Endpoint) (r *Relation, err error) {
	key := relationKey(eps)
	defer trivial.ErrorContextf(&err, "cannot add relation %q", key)
	// Enforce basic endpoint sanity. The epCount restrictions may be relaxed
	// in the future; if so, this method is likely to need significant rework.
	if len(eps) != 2 {
		return nil, fmt.Errorf("relation must have two endpoints")
	}
	if !eps[0].CanRelateTo(eps[1]) {
		return nil, fmt.Errorf("endpoints do not relate")
	}
	// If either endpoint has container scope, so must the other.
	if eps[0].Scope == charm.ScopeContainer {
		eps[1].Scope = charm.ScopeContainer
	} else if eps[1].Scope == charm.ScopeContainer {
		eps[0].Scope = charm.ScopeContainer
	}
	// We only get a unique relation id once, to save on roundtrips. If it's
	// -1, we haven't got it yet (we don't get it at this stage, because we
	// still don't know whether it's sane to even attempt creation).
	id := -1
	// If a service's charm is upgraded while we're trying to add a relation,
	// we'll need to re-validate service sanity. This is probably relatively
	// rare, so we only try 3 times.
	for attempt := 0; attempt < 3; attempt++ {
		// Perform initial relation sanity check.
		if exists, err := isNotDead(st.relations, key); err != nil {
			return nil, err
		} else if exists {
			return nil, fmt.Errorf("relation already exists")
		}
		// Collect per-service operations, checking sanity as we go.
		var ops []txn.Op
		for _, ep := range eps {
			svc, err := st.Service(ep.ServiceName)
			if IsNotFound(err) {
				return nil, fmt.Errorf("service %q does not exist", ep.ServiceName)
			} else if err != nil {
				return nil, err
			} else if svc.doc.Life != Alive {
				return nil, fmt.Errorf("service %q is not alive", ep.ServiceName)
			}
			ch, _, err := svc.Charm()
			if err != nil {
				return nil, err
			}
			if !ep.ImplementedBy(ch) {
				return nil, fmt.Errorf("%q does not implement %q", ep.ServiceName, ep)
			}
			ops = append(ops, txn.Op{
				C:      st.services.Name,
				Id:     ep.ServiceName,
				Assert: D{{"life", Alive}, {"charmurl", ch.URL()}},
				Update: D{{"$inc", D{{"relationcount", 1}}}},
			})
		}
		// Create a new unique id if that has not already been done, and add
		// an operation to create the relation document.
		if id == -1 {
			var err error
			if id, err = st.sequence("relation"); err != nil {
				return nil, err
			}
		}
		doc := relationDoc{
			Key:       key,
			Id:        id,
			Endpoints: eps,
			Life:      Alive,
		}
		ops = append(ops, txn.Op{
			C:      st.relations.Name,
			Id:     doc.Key,
			Assert: txn.DocMissing,
			Insert: doc,
		})
		// Run the transaction, and retry on abort.
		if err = st.runner.Run(ops, "", nil); err == txn.ErrAborted {
			continue
		} else if err != nil {
			return nil, err
		}
		return &Relation{st, doc}, nil
	}
	return nil, ErrExcessiveContention
}

// EndpointsRelation returns the existing relation with the given endpoints.
func (st *State) EndpointsRelation(endpoints ...Endpoint) (*Relation, error) {
	doc := relationDoc{}
	key := relationKey(endpoints)
	err := st.relations.Find(D{{"_id", key}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("relation %q", key)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get relation %q: %v", key, err)
	}
	return newRelation(st, &doc), nil
}

// Relation returns the existing relation with the given id.
func (st *State) Relation(id int) (*Relation, error) {
	doc := relationDoc{}
	err := st.relations.Find(D{{"id", id}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("relation %d", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get relation %d: %v", id, err)
	}
	return newRelation(st, &doc), nil
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	if !IsUnitName(name) {
		return nil, fmt.Errorf("%q is not a valid unit name", name)
	}
	doc := unitDoc{}
	err := st.units.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("unit %q", name)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get unit %q: %v", name, err)
	}
	return newUnit(st, &doc), nil
}

// DestroyUnits destroys the units with the specified names.
func (st *State) DestroyUnits(names ...string) (err error) {
	// TODO(rog) make this a transaction?
	var errs []string
	for _, name := range names {
		unit, err := st.Unit(name)
		switch {
		case IsNotFound(err):
			err = fmt.Errorf("unit %q does not exist", name)
		case err != nil:
		case unit.Life() != Alive:
			continue
		case unit.IsPrincipal():
			err = unit.Destroy()
		default:
			err = fmt.Errorf("unit %q is a subordinate", name)
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return destroyErr("units", names, errs)
}

// DestroyMachines destroys the machines with the specified ids.
func (st *State) DestroyMachines(ids ...string) (err error) {
	var errs []string
	for _, id := range ids {
		machine, err := st.Machine(id)
		switch {
		case IsNotFound(err):
			err = fmt.Errorf("machine %s does not exist", id)
		case err != nil:
		case machine.Life() != Alive:
			continue
		default:
			err = machine.Destroy()
		}
		if err != nil {
			errs = append(errs, err.Error())
		}
	}
	return destroyErr("machines", ids, errs)
}

func destroyErr(desc string, ids, errs []string) error {
	if len(errs) == 0 {
		return nil
	}
	msg := "some %s were not destroyed"
	if len(errs) == len(ids) {
		msg = "no %s were destroyed"
	}
	msg = fmt.Sprintf(msg, desc)
	return fmt.Errorf("%s: %s", msg, strings.Join(errs, "; "))
}

// AssignUnit places the unit on a machine. Depending on the policy, and the
// state of the environment, this may lead to new instances being launched
// within the environment.
func (st *State) AssignUnit(u *Unit, policy AssignmentPolicy) (err error) {
	if !u.IsPrincipal() {
		return fmt.Errorf("subordinate unit %q cannot be assigned directly to a machine", u)
	}
	defer trivial.ErrorContextf(&err, "cannot assign unit %q to machine", u)
	var m *Machine
	switch policy {
	case AssignLocal:
		m, err = st.Machine("0")
		if err != nil {
			return err
		}
		return u.AssignToMachine(m)
	case AssignUnused:
		if _, err = u.AssignToUnusedMachine(); err != noUnusedMachines {
			return err
		}
		return u.AssignToNewMachine()
	case AssignNew:
		return u.AssignToNewMachine()
	}
	panic(fmt.Errorf("unknown unit assignment policy: %q", policy))
}

// StartSync forces watchers to resynchronize their state with the
// database immediately. This will happen periodically automatically.
func (st *State) StartSync() {
	st.watcher.StartSync()
	st.pwatcher.StartSync()
}

// Sync forces watchers to resynchronize their state with the
// database immediately, and waits until all events are known.
func (st *State) Sync() {
	st.watcher.Sync()
	st.pwatcher.Sync()
}

// SetAdminMongoPassword sets the administrative password
// to access the state. If the password is non-empty,
// all subsequent attempts to access the state must
// be authorized; otherwise no authorization is required.
func (st *State) SetAdminMongoPassword(password string) error {
	admin := st.db.Session.DB("admin")
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

func (st *State) setMongoPassword(name, password string) error {
	if err := st.db.AddUser(name, password, false); err != nil {
		return fmt.Errorf("cannot set password in juju db for %q: %v", name, err)
	}
	if err := st.db.Session.DB("presence").AddUser(name, password, false); err != nil {
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
func (st *State) Cleanup() error {
	doc := cleanupDoc{}
	iter := st.cleanups.Find(nil).Iter()
	for iter.Next(&doc) {
		var c *mgo.Collection
		var sel interface{}
		switch doc.Kind {
		case "settings":
			c = st.settings
			sel = D{{"_id", D{{"$regex", "^" + doc.Prefix}}}}
		default:
			log.Warningf("state: ignoring unknown cleanup kind %q", doc.Kind)
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
			C:      st.cleanups.Name,
			Id:     doc.Id,
			Remove: true,
		}}
		if err := st.runner.Run(ops, "", nil); err != nil {
			return fmt.Errorf("cannot remove empty cleanup document: %v", err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read cleanup document: %v", err)
	}
	return nil
}
