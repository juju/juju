// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The state package enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"labix.org/v2/mgo/txn"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/multiwatcher"
	"launchpad.net/juju-core/state/presence"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var logger = loggo.GetLogger("juju.state")

// TODO(niemeyer): This must not be exported.
type D []bson.DocElem

// BootstrapNonce is used as a nonce for the state server machine.
const BootstrapNonce = "user-admin:bootstrap"

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
	stateServers     *mgo.Collection
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

// Ping probes the state's database connection to ensure
// that it is still alive.
func (st *State) Ping() error {
	return st.db.Session.Ping()
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
	return config.New(config.NoDefaults, attrs)
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

// versionInconsistentError indicates one or more agents have a
// different version from the current one (even empty, when not yet
// set).
type versionInconsistentError struct {
	currentVersion version.Number
	agents         []string
}

func (e *versionInconsistentError) Error() string {
	sort.Strings(e.agents)
	return fmt.Sprintf("some agents have not upgraded to the current environment version %s: %s", e.currentVersion, strings.Join(e.agents, ", "))
}

// newVersionInconsistentError returns a new instance of
// versionInconsistentError.
func newVersionInconsistentError(currentVersion version.Number, agents []string) *versionInconsistentError {
	return &versionInconsistentError{currentVersion, agents}
}

// IsVersionInconsistentError returns if the given error is
// versionInconsistentError.
func IsVersionInconsistentError(e interface{}) bool {
	_, ok := e.(*versionInconsistentError)
	return ok
}

func (st *State) checkCanUpgrade(currentVersion, newVersion string) error {
	matchCurrent := "^" + regexp.QuoteMeta(currentVersion) + "-"
	matchNew := "^" + regexp.QuoteMeta(newVersion) + "-"
	// Get all machines and units with a different or empty version.
	sel := D{{"$or", []D{
		{{"tools", D{{"$exists", false}}}},
		{{"$and", []D{
			{{"tools.version", D{{"$not", bson.RegEx{matchCurrent, ""}}}}},
			{{"tools.version", D{{"$not", bson.RegEx{matchNew, ""}}}}},
		}}},
	}}}
	var agentTags []string
	for _, collection := range []*mgo.Collection{st.machines, st.units} {
		var doc struct {
			Id string `bson:"_id"`
		}
		iter := collection.Find(sel).Select(D{{"_id", 1}}).Iter()
		for iter.Next(&doc) {
			switch collection.Name {
			case "machines":
				agentTags = append(agentTags, names.MachineTag(doc.Id))
			case "units":
				agentTags = append(agentTags, names.UnitTag(doc.Id))
			}
		}
		if err := iter.Err(); err != nil {
			return err
		}
	}
	if len(agentTags) > 0 {
		return newVersionInconsistentError(version.MustParse(currentVersion), agentTags)
	}
	return nil
}

// SetEnvironAgentVersion changes the agent version for the
// environment to the given version, only if the environment is in a
// stable state (all agents are running the current version).
func (st *State) SetEnvironAgentVersion(newVersion version.Number) error {
	for i := 0; i < 5; i++ {
		settings, err := readSettings(st, environGlobalKey)
		if err != nil {
			return err
		}
		agentVersion, ok := settings.Get("agent-version")
		if !ok {
			return fmt.Errorf("no agent version set in the environment")
		}
		currentVersion, ok := agentVersion.(string)
		if !ok {
			return fmt.Errorf("invalid agent version format: expected string, got %v", agentVersion)
		}
		if newVersion.String() == currentVersion {
			// Nothing to do.
			return nil
		}

		if err := st.checkCanUpgrade(currentVersion, newVersion.String()); err != nil {
			return err
		}

		ops := []txn.Op{{
			C:      st.settings.Name,
			Id:     environGlobalKey,
			Assert: D{{"txn-revno", settings.txnRevno}},
			Update: D{{"$set", D{{"agent-version", newVersion.String()}}}},
		}}
		if err := st.runTransaction(ops); err == nil {
			return nil
		} else if err != txn.ErrAborted {
			return fmt.Errorf("cannot set agent-version: %v", err)
		}
	}
	return ErrExcessiveContention
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
	if params.InstanceId != "" {
		return nil, fmt.Errorf("cannot specify an instance id when adding a new machine")
	}
	if params.Nonce != "" {
		return nil, fmt.Errorf("cannot specify a nonce when adding a new machine")
	}

	// TODO(wallyworld) - if a container is required, and when the actual machine characteristics
	// are made available, we need to check the machine constraints to ensure the container can be
	// created on the specifed machine.
	// ie it makes no sense asking for a 16G container on a machine with 8G.

	return st.addMachine(params)
}

// InjectMachine adds a new machine, corresponding to an existing provider
// instance, configured according to the supplied params struct.
func (st *State) InjectMachine(params *AddMachineParams) (m *Machine, err error) {
	if params.InstanceId == "" {
		return nil, fmt.Errorf("cannot inject a machine without an instance id")
	}
	if params.Nonce == "" {
		return nil, fmt.Errorf("cannot inject a machine without a nonce")
	}
	return st.addMachine(params)
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
	Series                  string
	Constraints             constraints.Value
	ParentId                string
	ContainerType           instance.ContainerType
	InstanceId              instance.Id
	HardwareCharacteristics instance.HardwareCharacteristics
	Nonce                   string
	Jobs                    []MachineJob
}

// addMachineContainerOps returns txn operations and associated Mongo records used to create a new machine,
// accounting for the fact that a machine may require a container and may require instance data.
// This method exists to cater for:
// 1. InjectMachine, which is used to record in state an instantiated bootstrap node. When adding
// a machine to state so that it is provisioned normally, the instance id is not known at this point.
// 2. AssignToNewMachine, which is used to create a new machine on which to deploy a unit.
func (st *State) addMachineContainerOps(params *AddMachineParams, cons constraints.Value) ([]txn.Op, *instanceData, *containerRefParams, error) {
	var instData *instanceData
	if params.InstanceId != "" {
		instData = &instanceData{
			InstanceId: params.InstanceId,
			Arch:       params.HardwareCharacteristics.Arch,
			Mem:        params.HardwareCharacteristics.Mem,
			RootDisk:   params.HardwareCharacteristics.RootDisk,
			CpuCores:   params.HardwareCharacteristics.CpuCores,
			CpuPower:   params.HardwareCharacteristics.CpuPower,
			Tags:       params.HardwareCharacteristics.Tags,
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
		mdoc.InstanceId = params.InstanceId
		mdoc.Nonce = params.Nonce
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

// FindEntity returns the entity with the given tag.
//
// The returned value can be of type *Machine, *Unit,
// *User, *Service or *Environment, depending
// on the tag.
func (st *State) FindEntity(tag string) (Entity, error) {
	kind, id, err := names.ParseTag(tag, "")
	switch kind {
	case names.MachineTagKind:
		return st.Machine(id)
	case names.UnitTagKind:
		return st.Unit(id)
	case names.UserTagKind:
		return st.User(id)
	case names.ServiceTagKind:
		return st.Service(id)
	case names.EnvironTagKind:
		conf, err := st.EnvironConfig()
		if err != nil {
			return nil, err
		}
		// Return an invalid entity error if the requested environment is not
		// the current one.
		if id != conf.Name() {
			return nil, errors.NotFoundf("environment %q", id)
		}
		return st.Environment()
	case names.RelationTagKind:
		return st.KeyRelation(id)
	}
	return nil, err
}

// parseTag, given an entity tag, returns the collection name and id
// of the entity document.
func (st *State) parseTag(tag string) (coll string, id string, err error) {
	kind, id, err := names.ParseTag(tag, "")
	if err != nil {
		return "", "", err
	}
	switch kind {
	case names.MachineTagKind:
		coll = st.machines.Name
	case names.ServiceTagKind:
		coll = st.services.Name
	case names.UnitTagKind:
		coll = st.units.Name
	case names.UserTagKind:
		coll = st.users.Name
	case names.RelationTagKind:
		coll = st.relations.Name
	default:
		return "", "", fmt.Errorf("%q is not a valid collection tag", tag)
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
	if !names.IsService(name) {
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
	if !names.IsService(name) {
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
	if !names.IsUnit(name) {
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

type stateServersDoc struct {
	Id         string `bson:"_id"`
	MachineIds []string
}

// StateServerMachineIds returns a sorted slice of the ids
// of all machines that are configured to run a state server.
func (st *State) StateServerMachineIds() ([]string, error) {
	var doc stateServersDoc
	err := st.stateServers.Find(D{{"_id", ""}}).One(&doc)
	if err != nil {
		return nil, fmt.Errorf("cannot get state servers document: %v", err)
	}
	sort.Strings(doc.MachineIds)
	return doc.MachineIds, nil
}

const maxMongoPeers = 7

// EnsureAvailability adds state server machines as necessary to make
// the number of live state servers equal to numStateServers. The given
// constraints will be attached to any new machines.
//
// TODO(rog):
// If any current state servers are down, they will be
// removed from the current set of state servers
// (although the machines themselves will remain).
func (st *State) EnsureAvailability(numStateServers int, cons constraints.Value) error {
	if numStateServers%2 != 1 {
		return fmt.Errorf("number of state servers must be odd")
	}
	if numStateServers > maxMongoPeers {
		return fmt.Errorf("state server count is too large (allowed %d)", maxMongoPeers)
	}
	machineIds, err := st.StateServerMachineIds()
	if err != nil {
		return err
	}
	if len(machineIds) == numStateServers {
		// TODO check for machines that are down.
		return nil
	}
	if len(machineIds) > numStateServers {
		return fmt.Errorf("cannot reduce state server count")
	}

}

// ResumeTransactions resumes all pending transactions.
func (st *State) ResumeTransactions() error {
	return st.runner.ResumeAll()
}

var tagPrefix = map[byte]string{
	'm': names.MachineTagKind + "-",
	's': names.ServiceTagKind + "-",
	'u': names.UnitTagKind + "-",
	'e': names.EnvironTagKind + "-",
	'r': names.RelationTagKind + "-",
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
