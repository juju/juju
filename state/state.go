// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/multiwatcher"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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
const numberSnippet = "(0|[1-9][0-9]*)"
const containerSnippet = "(/[a-z]+/" + numberSnippet + ")"

var (
	validService = regexp.MustCompile("^" + serviceSnippet + "$")
	validUnit    = regexp.MustCompile("^" + serviceSnippet + "/" + numberSnippet + "$")
	validMachine = regexp.MustCompile("^" + numberSnippet + containerSnippet + "*$")
)

// BootstrapNonce is used as a nonce for the state server machine.
const BootstrapNonce = "user-admin:bootstrap"

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

// State represents the state of an environment
// managed by juju.
type State struct {
	info             *Info
	db               *mgo.Database
	environments     *mgo.Collection
	charms           *mgo.Collection
	machines         *mgo.Collection
	instanceData     *mgo.Collection
	containerRefs    *mgo.Collection
	relations        *mgo.Collection
	relationScopes   *mgo.Collection
	services         *mgo.Collection
	minUnits         *mgo.Collection
	settings         *mgo.Collection
	settingsrefs     *mgo.Collection
	constraints      *mgo.Collection
	units            *mgo.Collection
	users            *mgo.Collection
	presence         *mgo.Collection
	cleanups         *mgo.Collection
	annotations      *mgo.Collection
	statuses         *mgo.Collection
	runner           *txn.Runner
	transactionHooks chan ([]transactionHook)
	watcher          *watcher.Watcher
	pwatcher         *presence.Watcher
	// mu guards allManager.
	mu         sync.Mutex
	allManager *multiwatcher.StoreManager
}

// transactionHook holds a pair of functions to be called before and after a
// mgo/txn transaction is run. It is only used in testing.
type transactionHook struct {
	Before func()
	After  func()
}

// runTransaction runs the supplied operations as a single mgo/txn transaction,
// and includes a mechanism whereby tests can use SetTransactionHooks to induce
// arbitrary state mutations before and after particular transactions.
func (st *State) runTransaction(ops []txn.Op) error {
	transactionHooks := <-st.transactionHooks
	st.transactionHooks <- nil
	if len(transactionHooks) > 0 {
		defer func() {
			if transactionHooks[0].After != nil {
				transactionHooks[0].After()
			}
			if <-st.transactionHooks != nil {
				panic("concurrent use of transaction hooks")
			}
			st.transactionHooks <- transactionHooks[1:]
		}()
		if transactionHooks[0].Before != nil {
			transactionHooks[0].Before()
		}
	}
	return st.runner.Run(ops, "", nil)
}

func (st *State) Watch() *multiwatcher.Watcher {
	st.mu.Lock()
	if st.allManager == nil {
		st.allManager = multiwatcher.NewStoreManager(newAllWatcherStateBacking(st))
	}
	st.mu.Unlock()
	return multiwatcher.NewWatcher(st.allManager)
}

func (st *State) EnvironConfig() (*config.Config, error) {
	settings, err := readSettings(st, environGlobalKey)
	if err != nil {
		return nil, err
	}
	attrs := settings.Map()
	return config.New(attrs)
}

// checkEnvironConfig returns an error if the config is definitely invalid.
func checkEnvironConfig(cfg *config.Config) error {
	if cfg.AdminSecret() != "" {
		return fmt.Errorf("admin-secret should never be written to the state")
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return fmt.Errorf("agent-version must always be set in state")
	}
	return nil
}

// SetEnvironConfig replaces the current configuration of the
// environment with the provided configuration.
func (st *State) SetEnvironConfig(cfg *config.Config) error {
	if err := checkEnvironConfig(cfg); err != nil {
		return err
	}
	// TODO(niemeyer): This isn't entirely right as the change is done as a
	// delta that the user didn't ask for. Instead, take a (old, new) config
	// pair, and apply *known* delta.
	settings, err := readSettings(st, environGlobalKey)
	if err != nil {
		return err
	}
	settings.Update(cfg.AllAttrs())
	_, err = settings.Write()
	return err
}

// EnvironConstraints returns the current environment constraints.
func (st *State) EnvironConstraints() (constraints.Value, error) {
	return readConstraints(st, environGlobalKey)
}

// SetEnvironConstraints replaces the current environment constraints.
func (st *State) SetEnvironConstraints(cons constraints.Value) error {
	return writeConstraints(st, environGlobalKey, cons)
}

// AddMachine adds a new machine configured to run the supplied jobs on the
// supplied series. The machine's constraints will be taken from the
// environment constraints.
func (st *State) AddMachine(series string, jobs ...MachineJob) (m *Machine, err error) {
	return st.addMachine(&AddMachineParams{Series: series, Jobs: jobs})
}

// AddMachineWithConstraints adds a new machine configured to run the supplied jobs on the
// supplied series. The machine's constraints and other configuration will be taken from
// the supplied params struct.
func (st *State) AddMachineWithConstraints(params *AddMachineParams) (m *Machine, err error) {

	// TODO(wallyworld) - if a container is required, and when the actual machine characteristics
	// are made available, we need to check the machine constraints to ensure the container can be
	// created on the specifed machine.
	// ie it makes no sense asking for a 16G container on a machine with 8G.

	return st.addMachine(params)
}

// InjectMachine adds a new machine, corresponding to an existing provider
// instance, configured to run the supplied jobs on the supplied series, using
// the specified constraints.
func (st *State) InjectMachine(series string, cons constraints.Value, instanceId instance.Id, jobs ...MachineJob) (m *Machine, err error) {
	if instanceId == "" {
		return nil, fmt.Errorf("cannot inject a machine without an instance id")
	}
	//TODO(wallyworld) - figure out how to determine the existing machine's characteristics so they can be recorded in state
	return st.addMachine(&AddMachineParams{Series: series, Constraints: cons, instanceId: instanceId, nonce: BootstrapNonce, Jobs: jobs})
}

// containerRefParams specify how a machineContainers document is to be created.
type containerRefParams struct {
	hostId      string
	newHost     bool
	hostOnly    bool
	containerId string
}

func (st *State) addMachineOps(mdoc *machineDoc, metadata *instanceData, cons constraints.Value, containerParams *containerRefParams) (*machineDoc, []txn.Op, error) {
	if mdoc.Series == "" {
		return nil, nil, fmt.Errorf("no series specified")
	}
	if len(mdoc.Jobs) == 0 {
		return nil, nil, fmt.Errorf("no jobs specified")
	}
	if containerParams.hostId != "" && mdoc.ContainerType == "" {
		return nil, nil, fmt.Errorf("no container type specified")
	}
	jset := make(map[MachineJob]bool)
	for _, j := range mdoc.Jobs {
		if jset[j] {
			return nil, nil, fmt.Errorf("duplicate job: %s", j)
		}
		jset[j] = true
	}
	if containerParams.hostId == "" {
		// we are creating a new machine instance (not a container).
		seq, err := st.sequence("machine")
		if err != nil {
			return nil, nil, err
		}
		mdoc.Id = strconv.Itoa(seq)
		containerParams.hostId = mdoc.Id
		containerParams.newHost = true
	}
	if mdoc.ContainerType != "" {
		// we are creating a container so set up a namespaced id.
		seq, err := st.sequence(fmt.Sprintf("machine%s%sContainer", containerParams.hostId, mdoc.ContainerType))
		if err != nil {
			return nil, nil, err
		}
		mdoc.Id = fmt.Sprintf("%s/%s/%d", containerParams.hostId, mdoc.ContainerType, seq)
		containerParams.containerId = mdoc.Id
	}
	mdoc.Life = Alive
	sdoc := statusDoc{
		Status: params.StatusPending,
	}
	// Machine constraints do not use a container constraint value.
	// Both provisioning and deployment constraints use the same constraints.Value struct
	// so here we clear the container value. Provisioning ignores the container value but
	// clearing it avoids potential confusion.
	cons.Container = nil
	ops := []txn.Op{
		{
			C:      st.machines.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: *mdoc,
		},
		createConstraintsOp(st, machineGlobalKey(mdoc.Id), cons),
		createStatusOp(st, machineGlobalKey(mdoc.Id), sdoc),
	}
	if metadata != nil {
		ops = append(ops, txn.Op{
			C:      st.instanceData.Name,
			Id:     mdoc.Id,
			Assert: txn.DocMissing,
			Insert: *metadata,
		})
	}
	ops = append(ops, createContainerRefOp(st, containerParams)...)
	return mdoc, ops, nil
}

// AddMachineParams encapsulates the parameters used to create a new machine.
type AddMachineParams struct {
	Series        string
	Constraints   constraints.Value
	ParentId      string
	ContainerType instance.ContainerType
	instanceId    instance.Id
	nonce         string
	Jobs          []MachineJob
}

// addMachineContainerOps returns txn operations and associated Mongo records used to create a new machine,
// accounting for the fact that a machine may require a container and may require instance data.
// This method exists to cater for:
// 1. InjectMachine, which is used to record in state an instantiated bootstrap node. When adding
// a machine to state so that it is provisioned normally, the instance id is not known at this point.
// 2. AssignToNewMachine, which is used to create a new machine on which to deploy a unit.
func (st *State) addMachineContainerOps(params *AddMachineParams, cons constraints.Value) ([]txn.Op, *instanceData, *containerRefParams, error) {
	var instData *instanceData
	if params.instanceId != "" {
		instData = &instanceData{
			InstanceId: params.instanceId,
		}
	}
	var ops []txn.Op
	var containerParams = &containerRefParams{hostId: params.ParentId, hostOnly: true}
	// If we are creating a container, first create the host (parent) machine if necessary.
	if params.ContainerType != "" {
		containerParams.hostOnly = false
		if params.ParentId == "" {
			// No parent machine is specified so create one.
			mdoc := &machineDoc{
				Series: params.Series,
				Jobs:   params.Jobs,
				Clean:  true,
			}
			mdoc, parentOps, err := st.addMachineOps(mdoc, instData, cons, &containerRefParams{})
			if err != nil {
				return nil, nil, nil, err
			}
			ops = parentOps
			containerParams.hostId = mdoc.Id
			containerParams.newHost = true
		} else {
			// If a parent machine is specified, make sure it exists.
			_, err := st.Machine(containerParams.hostId)
			if err != nil {
				return nil, nil, nil, err
			}
		}
	}
	return ops, instData, containerParams, nil
}

// addMachine implements AddMachine and InjectMachine.
func (st *State) addMachine(params *AddMachineParams) (m *Machine, err error) {
	msg := "cannot add a new machine"
	if params.ParentId != "" || params.ContainerType != "" {
		msg = "cannot add a new container"
	}
	defer utils.ErrorContextf(&err, msg)

	cons, err := st.EnvironConstraints()
	if err != nil {
		return nil, err
	}
	cons = params.Constraints.WithFallbacks(cons)

	ops, instData, containerParams, err := st.addMachineContainerOps(params, cons)
	if err != nil {
		return nil, err
	}
	mdoc := &machineDoc{
		Series:        params.Series,
		ContainerType: string(params.ContainerType),
		Jobs:          params.Jobs,
		Clean:         true,
	}
	if mdoc.ContainerType == "" {
		mdoc.InstanceId = params.instanceId
		mdoc.Nonce = params.nonce
	}
	mdoc, machineOps, err := st.addMachineOps(mdoc, instData, cons, containerParams)
	if err != nil {
		return nil, err
	}
	ops = append(ops, machineOps...)

	err = st.runTransaction(ops)
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

var errDead = fmt.Errorf("not found or dead")
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
	return machineIdLessThan(ms[i].Id, ms[j].Id)
}

// machineIdLessThan returns true if id1 < id2, false otherwise.
// Machine ids may include "/" separators if they are for a container so
// the comparison is done by comparing the id component values from
// left to right (most significant part to least significant). Ids for
// host machines are always less than ids for their containers.
func machineIdLessThan(id1, id2 string) bool {
	// Most times, we are dealing with host machines and not containers, so we will
	// try interpreting the ids as ints - this will be faster than dealing with the
	// container ids below.
	mint1, err1 := strconv.Atoi(id1)
	mint2, err2 := strconv.Atoi(id2)
	if err1 == nil && err2 == nil {
		return mint1 < mint2
	}
	// We have at least one container id so it gets complicated.
	idParts1 := strings.Split(id1, "/")
	idParts2 := strings.Split(id2, "/")
	nrParts1 := len(idParts1)
	nrParts2 := len(idParts2)
	minLen := nrParts1
	if nrParts2 < minLen {
		minLen = nrParts2
	}
	for x := 0; x < minLen; x++ {
		m1 := idParts1[x]
		m2 := idParts2[x]
		if m1 == m2 {
			continue
		}
		// See if the id part is a container type, and if so compare directly.
		if x%2 == 1 {
			return m1 < m2
		}
		// Compare the integer ids.
		// There's nothing we can do with errors at this point.
		mint1, _ := strconv.Atoi(m1)
		mint2, _ := strconv.Atoi(m2)
		return mint1 < mint2
	}
	return nrParts1 < nrParts2
}

// Machine returns the machine with the given id.
func (st *State) Machine(id string) (*Machine, error) {
	mdoc := &machineDoc{}
	sel := D{{"_id", id}}
	err := st.machines.Find(sel).One(mdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("machine %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("cannot get machine %s: %v", id, err)
	}
	return newMachine(st, mdoc), nil
}

// Tagger represents entities with a tag.
type Tagger interface {
	Tag() string
}

// Authenticator represents entites capable of handling password
// authentication.
type Authenticator interface {
	Refresh() error
	SetPassword(pass string) error
	PasswordValid(pass string) bool
}

// TaggedAuthenticator represents tagged entities capable of authentication.
type TaggedAuthenticator interface {
	Authenticator
	Tagger
}

// Annotator represents entities capable of handling annotations.
type Annotator interface {
	Annotation(key string) (string, error)
	Annotations() (map[string]string, error)
	SetAnnotations(pairs map[string]string) error
}

// TaggedAnnotator represents tagged entities capable of handling annotations.
type TaggedAnnotator interface {
	Annotator
	Tagger
}

// Authenticator attempts to return a TaggedAuthenticator with the given name.
func (st *State) Authenticator(name string) (TaggedAuthenticator, error) {
	e, err := st.entity(name)
	if err != nil {
		return nil, err
	}
	if e, ok := e.(TaggedAuthenticator); ok {
		return e, nil
	}
	return nil, fmt.Errorf("entity %q does not support authentication", name)
}

// Annotator attempts to return aa TaggedAnnotator with the given name.
func (st *State) Annotator(name string) (TaggedAnnotator, error) {
	e, err := st.entity(name)
	if err != nil {
		return nil, err
	}
	if e, ok := e.(TaggedAnnotator); ok {
		return e, nil
	}
	return nil, fmt.Errorf("entity %q does not support annotations", name)
}

// entity returns the entity for the given tag.
func (st *State) entity(tag string) (interface{}, error) {
	i := strings.Index(tag, "-")
	if i <= 0 || i >= len(tag)-1 {
		return nil, fmt.Errorf("invalid entity tag %q", tag)
	}
	prefix, id := tag[0:i], tag[i+1:]
	switch prefix {
	case "machine":
		id = MachineIdFromTag(tag)
		if !IsMachineId(id) {
			return nil, fmt.Errorf("invalid entity tag %q", tag)
		}
		return st.Machine(id)
	case "unit":
		i := strings.LastIndex(id, "-")
		if i == -1 {
			return nil, fmt.Errorf("invalid entity tag %q", tag)
		}
		name := id[:i] + "/" + id[i+1:]
		if !IsUnitName(name) {
			return nil, fmt.Errorf("invalid entity tag %q", tag)
		}
		return st.Unit(name)
	case "user":
		return st.User(id)
	case "service":
		if !IsServiceName(id) {
			return nil, fmt.Errorf("invalid entity tag %q", tag)
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
			return nil, fmt.Errorf("invalid entity tag %q", tag)
		}
		return st.Environment()
	}
	return nil, fmt.Errorf("invalid entity tag %q", tag)
}

// ParseTag, given an entity tag, returns the collection name and id
// of the entity document.
func (st *State) ParseTag(tag string) (string, string, error) {
	parts := strings.SplitN(tag, "-", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid entity name %q", tag)
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
			return "", "", fmt.Errorf("invalid entity name %q", tag)
		}
		id = id[:idx] + "/" + id[idx+1:]
	case "user":
		coll = st.users.Name
	default:
		return "", "", fmt.Errorf("invalid entity name %q", tag)
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
		return nil, errors.NotFoundf("charm %q", curl)
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
	defer utils.ErrorContextf(&err, "cannot add service %q", name)
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
		Series:        ch.URL().Series,
		Subordinate:   ch.Meta().Subordinate,
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
		}, {
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
	if err := st.runTransaction(ops); err == txn.ErrAborted {
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
		return nil, errors.NotFoundf("service %q", name)
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
	defer utils.ErrorContextf(&err, "cannot add relation %q", key)
	// Enforce basic endpoint sanity. The epCount restrictions may be relaxed
	// in the future; if so, this method is likely to need significant rework.
	if len(eps) != 2 {
		return nil, fmt.Errorf("relation must have two endpoints")
	}
	if !eps[0].CanRelateTo(eps[1]) {
		return nil, fmt.Errorf("endpoints do not relate")
	}
	// If either endpoint has container scope, so must the other; and the
	// services's series must also match, because they'll be deployed to
	// the same machines.
	matchSeries := true
	if eps[0].Scope == charm.ScopeContainer {
		eps[1].Scope = charm.ScopeContainer
	} else if eps[1].Scope == charm.ScopeContainer {
		eps[0].Scope = charm.ScopeContainer
	} else {
		matchSeries = false
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
		series := map[string]bool{}
		for _, ep := range eps {
			svc, err := st.Service(ep.ServiceName)
			if errors.IsNotFoundError(err) {
				return nil, fmt.Errorf("service %q does not exist", ep.ServiceName)
			} else if err != nil {
				return nil, err
			} else if svc.doc.Life != Alive {
				return nil, fmt.Errorf("service %q is not alive", ep.ServiceName)
			}
			series[svc.doc.Series] = true
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
		if matchSeries && len(series) != 1 {
			return nil, fmt.Errorf("principal and subordinate services' series must match")
		}
		// Create a new unique id if that has not already been done, and add
		// an operation to create the relation document.
		if id == -1 {
			var err error
			if id, err = st.sequence("relation"); err != nil {
				return nil, err
			}
		}
		doc := &relationDoc{
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
		if err = st.runTransaction(ops); err == txn.ErrAborted {
			continue
		} else if err != nil {
			return nil, err
		}
		return &Relation{st, *doc}, nil
	}
	return nil, ErrExcessiveContention
}

// EndpointsRelation returns the existing relation with the given endpoints.
func (st *State) EndpointsRelation(endpoints ...Endpoint) (*Relation, error) {
	return st.KeyRelation(relationKey(endpoints))
}

// KeyRelation returns the existing relation with the given key (which can
// be derived unambiguously from the relation's endpoints).
func (st *State) KeyRelation(key string) (*Relation, error) {
	doc := relationDoc{}
	err := st.relations.Find(D{{"_id", key}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("relation %q", key)
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
		return nil, errors.NotFoundf("relation %d", id)
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
		return nil, errors.NotFoundf("unit %q", name)
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
		case errors.IsNotFoundError(err):
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
		case errors.IsNotFoundError(err):
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
	defer utils.ErrorContextf(&err, "cannot assign unit %q to machine", u)
	var m *Machine
	switch policy {
	case AssignLocal:
		m, err = st.Machine("0")
		if err != nil {
			return err
		}
		return u.AssignToMachine(m)
	case AssignClean:
		if _, err = u.AssignToCleanMachine(); err != noCleanMachines {
			return err
		}
		return u.AssignToNewMachineOrContainer()
	case AssignCleanEmpty:
		if _, err = u.AssignToCleanEmptyMachine(); err != noCleanMachines {
			return err
		}
		return u.AssignToNewMachineOrContainer()
	case AssignNew:
		return u.AssignToNewMachine()
	}
	return fmt.Errorf("unknown unit assignment policy: %q", policy)
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

// newCleanupOp returns a txn.Op that creates a cleanup document with a unique
// id and the supplied kind and prefix.
func (st *State) newCleanupOp(kind, prefix string) txn.Op {
	doc := &cleanupDoc{
		Id:     bson.NewObjectId(),
		Kind:   kind,
		Prefix: prefix,
	}
	return txn.Op{
		C:      st.cleanups.Name,
		Id:     doc.Id,
		Insert: doc,
	}
}

// NeedsCleanup returns true if documents previously marked for removal exist.
func (st *State) NeedsCleanup() (bool, error) {
	count, err := st.cleanups.Count()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Cleanup removes all documents that were previously marked for removal, if
// any such exist. It should be called periodically by at least one element
// of the system.
func (st *State) Cleanup() error {
	doc := cleanupDoc{}
	iter := st.cleanups.Find(nil).Iter()
	for iter.Next(&doc) {
		var err error
		switch doc.Kind {
		case "settings":
			err = st.cleanupSettings(doc.Prefix)
		case "units":
			err = st.cleanupUnits(doc.Prefix)
		default:
			err = fmt.Errorf("unknown cleanup kind %q", doc.Kind)
		}
		if err != nil {
			log.Warningf("cleanup failed: %v", err)
			continue
		}
		ops := []txn.Op{{
			C:      st.cleanups.Name,
			Id:     doc.Id,
			Remove: true,
		}}
		if err := st.runTransaction(ops); err != nil {
			return fmt.Errorf("cannot remove empty cleanup document: %v", err)
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read cleanup document: %v", err)
	}
	return nil
}

func (st *State) cleanupSettings(prefix string) error {
	// Documents marked for cleanup are not otherwise referenced in the
	// system, and will not be under watch, and are therefore safe to
	// delete directly.
	sel := D{{"_id", D{{"$regex", "^" + prefix}}}}
	if count, err := st.settings.Find(sel).Count(); err != nil {
		return fmt.Errorf("cannot detect cleanup targets: %v", err)
	} else if count != 0 {
		if _, err := st.settings.RemoveAll(sel); err != nil {
			return fmt.Errorf("cannot remove documents marked for cleanup: %v", err)
		}
	}
	return nil
}

func (st *State) cleanupUnits(prefix string) error {
	// This won't miss units, because a Dying service cannot have units added
	// to it. But we do have to remove the units themselves via individual
	// transactions, because they could be in any state at all.
	unit := &Unit{st: st}
	sel := D{{"_id", D{{"$regex", "^" + prefix}}}, {"life", Alive}}
	iter := st.units.Find(sel).Iter()
	for iter.Next(&unit.doc) {
		if err := unit.Destroy(); err != nil {
			return err
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("cannot read unit document: %v", err)
	}
	return nil
}

// ResumeTransactions resumes all pending transactions.
func (st *State) ResumeTransactions() error {
	return st.runner.ResumeAll()
}

var tagPrefix = map[byte]string{
	'm': "machine-",
	's': "service-",
	'u': "unit-",
	'e': "environment-",
}

func tagForGlobalKey(key string) (string, bool) {
	if len(key) < 3 || key[1] != '#' {
		return "", false
	}
	p, ok := tagPrefix[key[0]]
	if !ok {
		return "", false
	}
	return p + key[2:], true
}
