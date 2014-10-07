// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v4"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.state")

const (
	// The following define the mongo collections used to record the Juju environment state.
	environmentsC      = "environments"
	charmsC            = "charms"
	machinesC          = "machines"
	containerRefsC     = "containerRefs"
	instanceDataC      = "instanceData"
	relationsC         = "relations"
	relationScopesC    = "relationscopes"
	servicesC          = "services"
	requestedNetworksC = "requestednetworks"
	networksC          = "networks"
	networkInterfacesC = "networkinterfaces"
	minUnitsC          = "minunits"
	settingsC          = "settings"
	settingsrefsC      = "settingsrefs"
	constraintsC       = "constraints"
	unitsC             = "units"
	actionsC           = "actions"
	actionresultsC     = "actionresults"
	usersC             = "users"
	envUsersC          = "envusers"
	presenceC          = "presence"
	cleanupsC          = "cleanups"
	annotationsC       = "annotations"
	statusesC          = "statuses"
	stateServersC      = "stateServers"
	openedPortsC       = "openedPorts"
	metricsC           = "metrics"
	upgradeInfoC       = "upgradeInfo"
	rebootC            = "reboot"

	// meterStatusC is the collection used to store meter status information.
	meterStatusC = "meterStatus"

	// toolsmetadataC is the collection used to store tools metadata.
	toolsmetadataC = "toolsmetadata"

	// This collection is used just for storing metadata.
	backupsMetaC = "backupsmetadata"

	// These collections are used by the mgo transaction runner.
	txnLogC = "txns.log"
	txnsC   = "txns"

	// blobstoreDB is the name of the blobstore GridFS database.
	blobstoreDB = "blobstore"
)

// State represents the state of an environment
// managed by juju.
type State struct {
	// transactionRunner is normally nil, which means that a new one
	// will be created for each operation, ensuring a fresh mgo.Session
	// is used. However, for tests, a value may be assigned and this will
	// be used instead of creating a new runnner each time.
	transactionRunner jujutxn.Runner
	mongoInfo         *mongo.MongoInfo
	policy            Policy
	db                *mgo.Database
	watcher           *watcher.Watcher
	pwatcher          *presence.Watcher
	// mu guards allManager.
	mu         sync.Mutex
	allManager *multiwatcher.StoreManager
	environTag names.EnvironTag
}

// StateServingInfo holds information needed by a state server.
// This type is a copy of the type of the same name from the api/params package.
// It is replicated here to avoid the state pacakge depending on api/params.
type StateServingInfo struct {
	APIPort    int
	StatePort  int
	Cert       string
	PrivateKey string
	// this will be passed as the KeyFile argument to MongoDB
	SharedSecret   string
	SystemIdentity string
}

// ForEnviron returns a connection to mongo for the specified environment. The
// connection uses the same credentails and policy as the existing connection.
func (st *State) ForEnviron(env names.EnvironTag) (*State, error) {
	newState, err := open(st.mongoInfo, mongo.DialOpts{}, st.policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newState.environTag = env
	return newState, nil
}

// EnvironTag() returns the environment tag for the environment controlled by
// this state instance.
func (st *State) EnvironTag() names.EnvironTag {
	return st.environTag
}

// getCollection fetches a named collection using a new session if the
// database has previously been logged in to.
// It returns the collection and a closer function for the session.
func (st *State) getCollection(coll string) (*mgo.Collection, func()) {
	return mongo.CollectionFromName(st.db, coll)
}

// getPresence returns the presence collection.
func (st *State) getPresence() *mgo.Collection {
	return st.db.Session.DB("presence").C(presenceC)
}

// newDB returns a database connection using a new session, along with
// a closer function for the session. This is useful where you need to work
// with various collections in a single session, so don't want to call
// getCollection multiple times.
func (st *State) newDB() (*mgo.Database, func()) {
	session := st.db.Session.Copy()
	return st.db.With(session), session.Close
}

// Ping probes the state's database connection to ensure
// that it is still alive.
func (st *State) Ping() error {
	return st.db.Session.Ping()
}

// MongoSession returns the underlying mongodb session
// used by the state. It is exposed so that external code
// can maintain the mongo replica set and should not
// otherwise be used.
func (st *State) MongoSession() *mgo.Session {
	return st.db.Session
}

type closeFunc func()

// txnRunner returns a jujutxn.Runner instance.
//
// If st.transactionRunner is non-nil, then that will be
// returned and the session argument will be ignored. This
// is the case in tests only, when we want to test concurrent
// operations.
//
// If st.transactionRunner is nil, then we create a new
// transaction runner with the provided session and return
// that.
func (st *State) txnRunner(session *mgo.Session) jujutxn.Runner {
	if st.transactionRunner != nil {
		return st.transactionRunner
	}
	runnerDb := st.db.With(session)
	return jujutxn.NewRunner(jujutxn.RunnerParams{Database: runnerDb})
}

// runTransaction is a convenience method delegating to transactionRunner.
func (st *State) runTransaction(ops []txn.Op) error {
	session := st.db.Session.Copy()
	defer session.Close()
	return st.txnRunner(session).RunTransaction(ops)
}

// run is a convenience method delegating to transactionRunner.
func (st *State) run(transactions jujutxn.TransactionSource) error {
	session := st.db.Session.Copy()
	defer session.Close()
	return st.txnRunner(session).Run(transactions)
}

// ResumeTransactions resumes all pending transactions.
func (st *State) ResumeTransactions() error {
	session := st.db.Session.Copy()
	defer session.Close()
	return st.txnRunner(session).ResumeTransactions()
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
		return nil, errors.Trace(err)
	}
	attrs := settings.Map()
	return config.New(config.NoDefaults, attrs)
}

// checkEnvironConfig returns an error if the config is definitely invalid.
func checkEnvironConfig(cfg *config.Config) error {
	if cfg.AdminSecret() != "" {
		return errors.Errorf("admin-secret should never be written to the state")
	}
	if _, ok := cfg.AgentVersion(); !ok {
		return errors.Errorf("agent-version must always be set in state")
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
	value := e
	// In case of a wrapped error, check the cause first.
	cause := errors.Cause(e.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*versionInconsistentError)
	return ok
}

func (st *State) checkCanUpgrade(currentVersion, newVersion string) error {
	db, closer := st.newDB()
	defer closer()

	matchCurrent := "^" + regexp.QuoteMeta(currentVersion) + "-"
	matchNew := "^" + regexp.QuoteMeta(newVersion) + "-"
	// Get all machines and units with a different or empty version.
	sel := bson.D{{"$or", []bson.D{
		{{"tools", bson.D{{"$exists", false}}}},
		{{"$and", []bson.D{
			{{"tools.version", bson.D{{"$not", bson.RegEx{matchCurrent, ""}}}}},
			{{"tools.version", bson.D{{"$not", bson.RegEx{matchNew, ""}}}}},
		}}},
	}}}
	var agentTags []string
	for _, name := range []string{machinesC, unitsC} {
		collection := db.C(name)

		var doc struct {
			Id string `bson:"_id"`
		}
		iter := collection.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
		for iter.Next(&doc) {
			switch name {
			case machinesC:
				agentTags = append(agentTags, names.NewMachineTag(doc.Id).String())
			case unitsC:
				unitName := st.localID(doc.Id)
				agentTags = append(agentTags, names.NewUnitTag(unitName).String())
			}
		}
		if err := iter.Close(); err != nil {
			return errors.Trace(err)
		}
	}
	if len(agentTags) > 0 {
		err := newVersionInconsistentError(version.MustParse(currentVersion), agentTags)
		return errors.Trace(err)
	}
	return nil
}

var UpgradeInProgressError = errors.New("an upgrade is already in progress or the last upgrade did not complete")

// IsUpgradeInProgressError returns true if the error given is UpgradeInProgressError.
func IsUpgradeInProgressError(err error) bool {
	return errors.Cause(err) == UpgradeInProgressError
}

// SetEnvironAgentVersion changes the agent version for the
// environment to the given version, only if the environment is in a
// stable state (all agents are running the current version).
func (st *State) SetEnvironAgentVersion(newVersion version.Number) (err error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		settings, err := readSettings(st, environGlobalKey)
		if err != nil {
			return nil, errors.Trace(err)
		}
		agentVersion, ok := settings.Get("agent-version")
		if !ok {
			return nil, errors.Errorf("no agent version set in the environment")
		}
		currentVersion, ok := agentVersion.(string)
		if !ok {
			return nil, errors.Errorf("invalid agent version format: expected string, got %v", agentVersion)
		}
		if newVersion.String() == currentVersion {
			// Nothing to do.
			return nil, jujutxn.ErrNoOperations
		}

		if err := st.checkCanUpgrade(currentVersion, newVersion.String()); err != nil {
			return nil, errors.Trace(err)
		}

		ops := []txn.Op{
			// Can't set agent-version if there's an active upgradeInfo doc.
			{
				C:      upgradeInfoC,
				Id:     currentUpgradeId,
				Assert: txn.DocMissing,
			}, {
				C:      settingsC,
				Id:     environGlobalKey,
				Assert: bson.D{{"txn-revno", settings.txnRevno}},
				Update: bson.D{{"$set", bson.D{{"agent-version", newVersion.String()}}}},
			},
		}
		return ops, nil
	}
	if err = st.run(buildTxn); err == jujutxn.ErrExcessiveContention {
		// Although there is a small chance of a race here, try to
		// return a more helpful error message in the case of an
		// active upgradeInfo document being in place.
		if upgrading, _ := st.IsUpgrading(); upgrading {
			err = UpgradeInProgressError
		} else {
			err = errors.Annotate(err, "cannot set agent version")
		}
	}
	return errors.Trace(err)
}

func (st *State) buildAndValidateEnvironConfig(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
	newConfig, err := oldConfig.Apply(updateAttrs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(removeAttrs) != 0 {
		newConfig, err = newConfig.Remove(removeAttrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := checkEnvironConfig(newConfig); err != nil {
		return nil, errors.Trace(err)
	}
	return st.validate(newConfig, oldConfig)
}

type ValidateConfigFunc func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error

// UpdateEnvironConfig adds, updates or removes attributes in the current
// configuration of the environment with the provided updateAttrs and
// removeAttrs.
func (st *State) UpdateEnvironConfig(updateAttrs map[string]interface{}, removeAttrs []string, additionalValidation ValidateConfigFunc) error {
	if len(updateAttrs)+len(removeAttrs) == 0 {
		return nil
	}

	// TODO(axw) 2013-12-6 #1167616
	// Ensure that the settings on disk have not changed
	// underneath us. The settings changes are actually
	// applied as a delta to what's on disk; if there has
	// been a concurrent update, the change may not be what
	// the user asked for.
	settings, err := readSettings(st, environGlobalKey)
	if err != nil {
		return errors.Trace(err)
	}

	// Get the existing environment config from state.
	oldConfig, err := config.New(config.NoDefaults, settings.Map())
	if err != nil {
		return errors.Trace(err)
	}
	if additionalValidation != nil {
		err = additionalValidation(updateAttrs, removeAttrs, oldConfig)
		if err != nil {
			return errors.Trace(err)
		}
	}
	validCfg, err := st.buildAndValidateEnvironConfig(updateAttrs, removeAttrs, oldConfig)
	if err != nil {
		return errors.Trace(err)
	}

	validAttrs := validCfg.AllAttrs()
	for k := range oldConfig.AllAttrs() {
		if _, ok := validAttrs[k]; !ok {
			settings.Delete(k)
		}
	}
	settings.Update(validAttrs)
	_, err = settings.Write()
	return errors.Trace(err)
}

// EnvironConstraints returns the current environment constraints.
func (st *State) EnvironConstraints() (constraints.Value, error) {
	cons, err := readConstraints(st, environGlobalKey)
	return cons, errors.Trace(err)
}

// SetEnvironConstraints replaces the current environment constraints.
func (st *State) SetEnvironConstraints(cons constraints.Value) error {
	unsupported, err := st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting environment constraints: unsupported constraints: %v", strings.Join(unsupported, ","))
	} else if err != nil {
		return errors.Trace(err)
	}
	return writeConstraints(st, environGlobalKey, cons)
}

var ErrDead = fmt.Errorf("not found or dead")
var errNotAlive = fmt.Errorf("not found or not alive")

func onAbort(txnErr, err error) error {
	if txnErr == txn.ErrAborted ||
		errors.Cause(txnErr) == txn.ErrAborted {
		return errors.Trace(err)
	}
	return errors.Trace(txnErr)
}

// AllMachines returns all machines in the environment
// ordered by id.
func (st *State) AllMachines() (machines []*Machine, err error) {
	machinesCollection, closer := st.getCollection(machinesC)
	defer closer()

	mdocs := machineDocSlice{}
	err = machinesCollection.Find(nil).All(&mdocs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all machines")
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
	machinesCollection, closer := st.getCollection(machinesC)
	defer closer()

	mdoc := &machineDoc{}
	sel := bson.D{{"_id", id}}
	err := machinesCollection.Find(sel).One(mdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("machine %s", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get machine %s", id)
	}
	return newMachine(st, mdoc), nil
}

// FindEntity returns the entity with the given tag.
//
// The returned value can be of type *Machine, *Unit,
// *User, *Service, *Environment, or *Action, depending
// on the tag.
func (st *State) FindEntity(tag names.Tag) (Entity, error) {
	id := tag.Id()
	switch tag := tag.(type) {
	case names.MachineTag:
		return st.Machine(id)
	case names.UnitTag:
		return st.Unit(id)
	case names.UserTag:
		return st.User(tag)
	case names.ServiceTag:
		return st.Service(id)
	case names.EnvironTag:
		env, err := st.Environment()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Return an invalid entity error if the requested environment is not
		// the current one.
		if id != env.UUID() {
			if utils.IsValidUUIDString(id) {
				return nil, errors.NotFoundf("environment %q", id)
			}
			// TODO(axw) 2013-12-04 #1257587
			// We should not accept environment tags that do not match the
			// environment's UUID. We accept anything for now, to cater
			// both for past usage, and for potentially supporting aliases.
			logger.Warningf("environment-tag does not match current environment UUID: %q != %q", id, env.UUID())
			conf, err := st.EnvironConfig()
			if err != nil {
				logger.Warningf("EnvironConfig failed: %v", err)
			} else if id != conf.Name() {
				logger.Warningf("environment-tag does not match current environment name: %q != %q", id, conf.Name())
			}
		}
		return env, nil
	case names.RelationTag:
		return st.KeyRelation(id)
	case names.NetworkTag:
		return st.Network(id)
	case names.ActionTag:
		return st.ActionByTag(tag)
	default:
		return nil, errors.Errorf("unsupported tag %T", tag)
	}
}

// parseTag, given an entity tag, returns the collection name and id
// of the entity document.
func (st *State) parseTag(tag names.Tag) (string, interface{}, error) {
	if tag == nil {
		return "", nil, errors.Errorf("tag is nil")
	}
	coll := ""
	id := tag.Id()
	switch tag := tag.(type) {
	case names.MachineTag:
		coll = machinesC
	case names.ServiceTag:
		coll = servicesC
		id = st.docID(id)
	case names.UnitTag:
		coll = unitsC
		id = st.docID(id)
	case names.UserTag:
		coll = usersC
		if !tag.IsLocal() {
			return "", nil, fmt.Errorf("%q is not a local user", tag.Username())
		}
		id = tag.Name()
	case names.RelationTag:
		coll = relationsC
	case names.EnvironTag:
		coll = environmentsC
	case names.NetworkTag:
		coll = networksC
	case names.ActionTag:
		coll = actionsC
		id = actionIdFromTag(tag)
	default:
		return "", nil, errors.Errorf("%q is not a valid collection tag", tag)
	}
	return coll, id, nil
}

// AddCharm adds the ch charm with curl to the state.
// On success the newly added charm state is returned.
func (st *State) AddCharm(ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string) (stch *Charm, err error) {
	// The charm may already exist in state as a placeholder, so we
	// check for that situation and update the existing charm record
	// if necessary, otherwise add a new record.
	var existing charmDoc
	charms, closer := st.getCollection(charmsC)
	defer closer()

	err = charms.Find(bson.D{{"_id", curl.String()}, {"placeholder", true}}).One(&existing)
	if err == mgo.ErrNotFound {
		cdoc := &charmDoc{
			URL:          curl,
			Meta:         ch.Meta(),
			Config:       ch.Config(),
			Actions:      ch.Actions(),
			BundleSha256: bundleSha256,
			StoragePath:  storagePath,
		}
		err = charms.Insert(cdoc)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot add charm %q", curl)
		}
		return newCharm(st, cdoc), nil
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return st.updateCharmDoc(ch, curl, storagePath, bundleSha256, stillPlaceholder)
}

// AllCharms returns all charms in state.
func (st *State) AllCharms() ([]*Charm, error) {
	charmsCollection, closer := st.getCollection(charmsC)
	defer closer()
	var cdoc charmDoc
	var charms []*Charm
	iter := charmsCollection.Find(nil).Iter()
	for iter.Next(&cdoc) {
		charms = append(charms, newCharm(st, &cdoc))
	}
	return charms, errors.Trace(iter.Close())
}

// Charm returns the charm with the given URL. Charms pending upload
// to storage and placeholders are never returned.
func (st *State) Charm(curl *charm.URL) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	cdoc := &charmDoc{}
	what := bson.D{
		{"_id", curl},
		{"placeholder", bson.D{{"$ne", true}}},
		{"pendingupload", bson.D{{"$ne", true}}},
	}
	err := charms.Find(what).One(&cdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get charm %q", curl)
	}
	if err := cdoc.Meta.Check(); err != nil {
		return nil, errors.Annotatef(err, "malformed charm metadata found in state")
	}
	return newCharm(st, cdoc), nil
}

// LatestPlaceholderCharm returns the latest charm described by the
// given URL but which is not yet deployed.
func (st *State) LatestPlaceholderCharm(curl *charm.URL) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(noRevURL.String())
	var docs []charmDoc
	err := charms.Find(bson.D{{"_id", bson.D{{"$regex", curlRegex}}}, {"placeholder", true}}).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get charm %q", curl)
	}
	// Find the highest revision.
	var latest charmDoc
	for _, doc := range docs {
		if latest.URL == nil || doc.URL.Revision > latest.URL.Revision {
			latest = doc
		}
	}
	if latest.URL == nil {
		return nil, errors.NotFoundf("placeholder charm %q", noRevURL)
	}
	return newCharm(st, &latest), nil
}

// PrepareLocalCharmUpload must be called before a local charm is
// uploaded to the provider storage in order to create a charm
// document in state. It returns the chosen unique charm URL reserved
// in state for the charm.
//
// The url's schema must be "local" and it must include a revision.
func (st *State) PrepareLocalCharmUpload(curl *charm.URL) (chosenUrl *charm.URL, err error) {
	// Perform a few sanity checks first.
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, errors.Errorf("expected charm URL with revision, got %q", curl)
	}
	// Get a regex with the charm URL and no revision.
	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(noRevURL.String())

	charms, closer := st.getCollection(charmsC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Find the highest revision of that charm in state.
		var docs []charmDoc
		err = charms.Find(bson.D{{"_id", bson.D{{"$regex", curlRegex}}}}).Select(bson.D{{"_id", 1}}).All(&docs)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Find the highest revision.
		maxRevision := -1
		for _, doc := range docs {
			if doc.URL.Revision > maxRevision {
				maxRevision = doc.URL.Revision
			}
		}

		// Respect the local charm's revision first.
		chosenRevision := curl.Revision
		if maxRevision >= chosenRevision {
			// More recent revision exists in state, pick the next.
			chosenRevision = maxRevision + 1
		}
		chosenUrl = curl.WithRevision(chosenRevision)

		uploadedCharm := &charmDoc{
			URL:           chosenUrl,
			PendingUpload: true,
		}
		ops := []txn.Op{{
			C:      charmsC,
			Id:     uploadedCharm.URL,
			Assert: txn.DocMissing,
			Insert: uploadedCharm,
		}}
		return ops, nil
	}
	if err = st.run(buildTxn); err == nil {
		return chosenUrl, nil
	}
	return nil, errors.Trace(err)
}

// PrepareStoreCharmUpload must be called before a charm store charm
// is uploaded to the provider storage in order to create a charm
// document in state. If a charm with the same URL is already in
// state, it will be returned as a *state.Charm (is can be still
// pending or already uploaded). Otherwise, a new charm document is
// added in state with just the given charm URL and
// PendingUpload=true, which is then returned as a *state.Charm.
//
// The url's schema must be "cs" and it must include a revision.
func (st *State) PrepareStoreCharmUpload(curl *charm.URL) (*Charm, error) {
	// Perform a few sanity checks first.
	if curl.Schema != "cs" {
		return nil, errors.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, errors.Errorf("expected charm URL with revision, got %q", curl)
	}

	charms, closer := st.getCollection(charmsC)
	defer closer()

	var (
		uploadedCharm charmDoc
		err           error
	)
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Find an uploaded or pending charm with the given exact curl.
		err := charms.FindId(curl).One(&uploadedCharm)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		} else if err == nil && !uploadedCharm.Placeholder {
			// The charm exists and it's either uploaded or still
			// pending, but it's not a placeholder. In any case,
			// there's nothing to do.
			return nil, jujutxn.ErrNoOperations
		} else if err == mgo.ErrNotFound {
			// Prepare the pending charm document for insertion.
			uploadedCharm = charmDoc{
				URL:           curl,
				PendingUpload: true,
				Placeholder:   false,
			}
		}

		var ops []txn.Op
		if uploadedCharm.Placeholder {
			// Convert the placeholder to a pending charm, while
			// asserting the fields updated after an upload have not
			// changed yet.
			ops = []txn.Op{{
				C:  charmsC,
				Id: curl,
				Assert: bson.D{
					{"bundlesha256", ""},
					{"pendingupload", false},
					{"placeholder", true},
				},
				Update: bson.D{{"$set", bson.D{
					{"pendingupload", true},
					{"placeholder", false},
				}}},
			}}
			// Update the fields of the document we're returning.
			uploadedCharm.PendingUpload = true
			uploadedCharm.Placeholder = false
		} else {
			// No charm document with this curl yet, insert it.
			ops = []txn.Op{{
				C:      charmsC,
				Id:     curl,
				Assert: txn.DocMissing,
				Insert: uploadedCharm,
			}}
		}
		return ops, nil
	}
	if err = st.run(buildTxn); err == nil {
		return newCharm(st, &uploadedCharm), nil
	}
	return nil, errors.Trace(err)
}

var (
	stillPending     = bson.D{{"pendingupload", true}}
	stillPlaceholder = bson.D{{"placeholder", true}}
)

// AddStoreCharmPlaceholder creates a charm document in state for the given charm URL which
// must reference a charm from the store. The charm document is marked as a placeholder which
// means that if the charm is to be deployed, it will need to first be uploaded to env storage.
func (st *State) AddStoreCharmPlaceholder(curl *charm.URL) (err error) {
	// Perform sanity checks first.
	if curl.Schema != "cs" {
		return errors.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return errors.Errorf("expected charm URL with revision, got %q", curl)
	}
	charms, closer := st.getCollection(charmsC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// See if the charm already exists in state and exit early if that's the case.
		var doc charmDoc
		err := charms.Find(bson.D{{"_id", curl.String()}}).Select(bson.D{{"_id", 1}}).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, errors.Trace(err)
		}
		if err == nil {
			return nil, jujutxn.ErrNoOperations
		}

		// Delete all previous placeholders so we don't fill up the database with unused data.
		ops, err := st.deleteOldPlaceholderCharmsOps(curl)
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Add the new charm doc.
		placeholderCharm := &charmDoc{
			URL:         curl,
			Placeholder: true,
		}
		ops = append(ops, txn.Op{
			C:      charmsC,
			Id:     placeholderCharm.URL.String(),
			Assert: txn.DocMissing,
			Insert: placeholderCharm,
		})
		return ops, nil
	}
	return errors.Trace(st.run(buildTxn))
}

// deleteOldPlaceholderCharmsOps returns the txn ops required to delete all placeholder charm
// records older than the specified charm URL.
func (st *State) deleteOldPlaceholderCharmsOps(curl *charm.URL) ([]txn.Op, error) {
	// Get a regex with the charm URL and no revision.
	noRevURL := curl.WithRevision(-1)
	curlRegex := "^" + regexp.QuoteMeta(noRevURL.String())

	charms, closer := st.getCollection(charmsC)
	defer closer()

	var docs []charmDoc
	err := charms.Find(
		bson.D{{"_id", bson.D{{"$regex", curlRegex}}}, {"placeholder", true}}).Select(bson.D{{"_id", 1}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	for _, doc := range docs {
		if doc.URL.Revision >= curl.Revision {
			continue
		}
		ops = append(ops, txn.Op{
			C:      charmsC,
			Id:     doc.URL.String(),
			Assert: stillPlaceholder,
			Remove: true,
		})
	}
	return ops, nil
}

// ErrCharmAlreadyUploaded is returned by UpdateUploadedCharm() when
// the given charm is already uploaded and marked as not pending in
// state.
type ErrCharmAlreadyUploaded struct {
	curl *charm.URL
}

func (e *ErrCharmAlreadyUploaded) Error() string {
	return fmt.Sprintf("charm %q already uploaded", e.curl)
}

// IsCharmAlreadyUploadedError returns if the given error is
// ErrCharmAlreadyUploaded.
func IsCharmAlreadyUploadedError(err interface{}) bool {
	if err == nil {
		return false
	}
	// In case of a wrapped error, check the cause first.
	value := err
	cause := errors.Cause(err.(error))
	if cause != nil {
		value = cause
	}
	_, ok := value.(*ErrCharmAlreadyUploaded)
	return ok
}

// ErrCharmRevisionAlreadyModified is returned when a pending or
// placeholder charm is no longer pending or a placeholder, signaling
// the charm is available in state with its full information.
var ErrCharmRevisionAlreadyModified = fmt.Errorf("charm revision already modified")

// UpdateUploadedCharm marks the given charm URL as uploaded and
// updates the rest of its data, returning it as *state.Charm.
func (st *State) UpdateUploadedCharm(ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	doc := &charmDoc{}
	err := charms.FindId(curl).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !doc.PendingUpload {
		return nil, errors.Trace(&ErrCharmAlreadyUploaded{curl})
	}

	return st.updateCharmDoc(ch, curl, storagePath, bundleSha256, stillPending)
}

// updateCharmDoc updates the charm with specified URL with the given
// data, and resets the placeholder and pendingupdate flags.  If the
// charm is no longer a placeholder or pending (depending on preReq),
// it returns ErrCharmRevisionAlreadyModified.
func (st *State) updateCharmDoc(
	ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string, preReq interface{}) (*Charm, error) {

	// Make sure we escape any "$" and "." in config option names
	// first. See http://pad.lv/1308146.
	cfg := ch.Config()
	escapedConfig := charm.NewConfig()
	for optionName, option := range cfg.Options {
		escapedName := escapeReplacer.Replace(optionName)
		escapedConfig.Options[escapedName] = option
	}
	updateFields := bson.D{{"$set", bson.D{
		{"meta", ch.Meta()},
		{"config", escapedConfig},
		{"actions", ch.Actions()},
		{"storagepath", storagePath},
		{"bundlesha256", bundleSha256},
		{"pendingupload", false},
		{"placeholder", false},
	}}}
	ops := []txn.Op{{
		C:      charmsC,
		Id:     curl,
		Assert: preReq,
		Update: updateFields,
	}}
	if err := st.runTransaction(ops); err != nil {
		return nil, onAbort(err, ErrCharmRevisionAlreadyModified)
	}
	return st.Charm(curl)
}

// addPeerRelationsOps returns the operations necessary to add the
// specified service peer relations to the state.
func (st *State) addPeerRelationsOps(serviceName string, peers map[string]charm.Relation) ([]txn.Op, error) {
	var ops []txn.Op
	for _, rel := range peers {
		relId, err := st.sequence("relation")
		if err != nil {
			return nil, errors.Trace(err)
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
			C:      relationsC,
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
func (st *State) AddService(name, owner string, ch *Charm, networks []string) (service *Service, err error) {
	defer errors.Maskf(&err, "cannot add service %q", name)
	ownerTag, err := names.ParseUserTag(owner)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid ownertag %s", owner)
	}
	// Sanity checks.
	if !names.IsValidService(name) {
		return nil, errors.Errorf("invalid name")
	}
	if ch == nil {
		return nil, errors.Errorf("charm is nil")
	}
	if exists, err := isNotDead(st.db, servicesC, name); err != nil {
		return nil, errors.Trace(err)
	} else if exists {
		return nil, errors.Errorf("service already exists")
	}
	env, err := st.Environment()
	if err != nil {
		return nil, errors.Trace(err)
	} else if env.Life() != Alive {
		return nil, errors.Errorf("environment is no longer alive")
	}
	if _, err := st.EnvironmentUser(ownerTag); err != nil {
		return nil, errors.Trace(err)
	}
	serviceID := st.docID(name)
	// Create the service addition operations.
	peers := ch.Meta().Peers
	svcDoc := &serviceDoc{
		DocID:         serviceID,
		Name:          name,
		EnvUUID:       env.UUID(),
		Series:        ch.URL().Series,
		Subordinate:   ch.Meta().Subordinate,
		CharmURL:      ch.URL(),
		RelationCount: len(peers),
		Life:          Alive,
		OwnerTag:      owner,
	}
	svc := newService(st, svcDoc)
	ops := []txn.Op{
		env.assertAliveOp(),
		createConstraintsOp(st, svc.globalKey(), constraints.Value{}),
		// TODO(dimitern) 2014-04-04 bug #1302498
		// Once we can add networks independently of machine
		// provisioning, we should check the given networks are valid
		// and known before setting them.
		createRequestedNetworksOp(st, svc.globalKey(), networks),
		createSettingsOp(st, svc.settingsKey(), nil),
		{
			C:      settingsrefsC,
			Id:     svc.settingsKey(),
			Assert: txn.DocMissing,
			Insert: settingsRefsDoc{1},
		},
		{
			C:      servicesC,
			Id:     serviceID,
			Assert: txn.DocMissing,
			Insert: svcDoc,
		}}
	// Collect peer relation addition operations.
	peerOps, err := st.addPeerRelationsOps(name, peers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, peerOps...)

	if err := st.runTransaction(ops); err == txn.ErrAborted {
		err := env.Refresh()
		if (err == nil && env.Life() != Alive) || errors.IsNotFound(err) {
			return nil, errors.Errorf("environment is no longer alive")
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		return nil, errors.Errorf("service already exists")
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	// Refresh to pick the txn-revno.
	if err = svc.Refresh(); err != nil {
		return nil, errors.Trace(err)
	}
	return svc, nil
}

// AddNetwork creates a new network with the given params. If a
// network with the same name or provider id already exists in state,
// an error satisfying errors.IsAlreadyExists is returned.
func (st *State) AddNetwork(args NetworkInfo) (n *Network, err error) {
	defer errors.Contextf(&err, "cannot add network %q", args.Name)
	if args.CIDR != "" {
		_, _, err := net.ParseCIDR(args.CIDR)
		if err != nil {
			return nil, err
		}
	}
	if args.Name == "" {
		return nil, errors.Errorf("name must be not empty")
	}
	if !names.IsValidNetwork(args.Name) {
		return nil, errors.Errorf("invalid name")
	}
	if args.ProviderId == "" {
		return nil, errors.Errorf("provider id must be not empty")
	}
	if args.VLANTag < 0 || args.VLANTag > 4094 {
		return nil, errors.Errorf("invalid VLAN tag %d: must be between 0 and 4094", args.VLANTag)
	}
	doc := newNetworkDoc(args)
	ops := []txn.Op{{
		C:      networksC,
		Id:     args.Name,
		Assert: txn.DocMissing,
		Insert: doc,
	}}
	err = st.runTransaction(ops)
	switch err {
	case txn.ErrAborted:
		if _, err = st.Network(args.Name); err == nil {
			return nil, errors.AlreadyExistsf("network %q", args.Name)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
	case nil:
		// We have a unique key restriction on the ProviderId field,
		// which will cause the insert to fail if there is another
		// record with the same provider id in the table. The txn
		// logic does not report insertion errors, so we check that
		// the record has actually been inserted correctly before
		// reporting success.
		if _, err = st.Network(args.Name); err != nil {
			return nil, errors.AlreadyExistsf("network with provider id %q", args.ProviderId)
		}
		return newNetwork(st, doc), nil
	}
	return nil, errors.Trace(err)
}

// Network returns the network with the given name.
func (st *State) Network(name string) (*Network, error) {
	networks, closer := st.getCollection(networksC)
	defer closer()

	doc := &networkDoc{}
	err := networks.FindId(name).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("network %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get network %q", name)
	}
	return newNetwork(st, doc), nil
}

// AllNetworks returns all known networks in the environment.
func (st *State) AllNetworks() (networks []*Network, err error) {
	networksCollection, closer := st.getCollection(networksC)
	defer closer()

	docs := []networkDoc{}
	err = networksCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all networks")
	}
	for _, doc := range docs {
		networks = append(networks, newNetwork(st, &doc))
	}
	return networks, nil
}

// Service returns a service state by name.
func (st *State) Service(name string) (service *Service, err error) {
	services, closer := st.getCollection(servicesC)
	defer closer()

	if !names.IsValidService(name) {
		return nil, errors.Errorf("%q is not a valid service name", name)
	}
	sdoc := &serviceDoc{}
	err = services.FindId(st.docID(name)).One(sdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("service %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get service %q", name)
	}
	return newService(st, sdoc), nil
}

// AllServices returns all deployed services in the environment.
func (st *State) AllServices() (services []*Service, err error) {
	servicesCollection, closer := st.getCollection(servicesC)
	defer closer()

	sdocs := []serviceDoc{}
	err = servicesCollection.Find(bson.D{}).All(&sdocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all services")
	}
	for _, v := range sdocs {
		services = append(services, newService(st, &v))
	}
	return services, nil
}

// docID generates a globally unique id value
// where the environment uuid is prefixed to the
// localID.
func (st *State) docID(localID string) string {
	return st.EnvironTag().Id() + ":" + localID
}

// localID returns the local id value by stripping
// of the environment uuid prefix if it is there.
func (st *State) localID(ID string) string {
	prefix := st.EnvironTag().Id() + ":"
	if strings.HasPrefix(ID, prefix) {
		return ID[len(prefix):]
	}
	return ID
}

// InferEndpoints returns the endpoints corresponding to the supplied names.
// There must be 1 or 2 supplied names, of the form <service>[:<relation>].
// If the supplied names uniquely specify a possible relation, or if they
// uniquely specify a possible relation once all implicit relations have been
// filtered, the endpoints corresponding to that relation will be returned.
func (st *State) InferEndpoints(names ...string) ([]Endpoint, error) {
	// Collect all possible sane endpoint lists.
	var candidates [][]Endpoint
	switch len(names) {
	case 1:
		eps, err := st.endpoints(names[0], isPeer)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, ep := range eps {
			candidates = append(candidates, []Endpoint{ep})
		}
	case 2:
		eps1, err := st.endpoints(names[0], notPeer)
		if err != nil {
			return nil, errors.Trace(err)
		}
		eps2, err := st.endpoints(names[1], notPeer)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for _, ep1 := range eps1 {
			for _, ep2 := range eps2 {
				if ep1.CanRelateTo(ep2) {
					candidates = append(candidates, []Endpoint{ep1, ep2})
				}
			}
		}
	default:
		return nil, errors.Errorf("cannot relate %d endpoints", len(names))
	}
	// If there's ambiguity, try discarding implicit relations.
	switch len(candidates) {
	case 0:
		return nil, errors.Errorf("no relations found")
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
	return nil, errors.Errorf("ambiguous relation: %q could refer to %s",
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
		return nil, errors.Errorf("invalid endpoint %q", name)
	}
	svc, err := st.Service(svcName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eps := []Endpoint{}
	if relName != "" {
		ep, err := svc.Endpoint(relName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		eps = append(eps, ep)
	} else {
		eps, err = svc.Endpoints()
		if err != nil {
			return nil, errors.Trace(err)
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
	defer errors.Maskf(&err, "cannot add relation %q", key)
	// Enforce basic endpoint sanity. The epCount restrictions may be relaxed
	// in the future; if so, this method is likely to need significant rework.
	if len(eps) != 2 {
		return nil, errors.Errorf("relation must have two endpoints")
	}
	if !eps[0].CanRelateTo(eps[1]) {
		return nil, errors.Errorf("endpoints do not relate")
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
	// we'll need to re-validate service sanity.
	var doc *relationDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Perform initial relation sanity check.
		if exists, err := isNotDead(st.db, relationsC, key); err != nil {
			return nil, errors.Trace(err)
		} else if exists {
			return nil, errors.Errorf("relation already exists")
		}
		// Collect per-service operations, checking sanity as we go.
		var ops []txn.Op
		series := map[string]bool{}
		for _, ep := range eps {
			svc, err := st.Service(ep.ServiceName)
			if errors.IsNotFound(err) {
				return nil, errors.Errorf("service %q does not exist", ep.ServiceName)
			} else if err != nil {
				return nil, errors.Trace(err)
			} else if svc.doc.Life != Alive {
				return nil, errors.Errorf("service %q is not alive", ep.ServiceName)
			}
			series[svc.doc.Series] = true
			ch, _, err := svc.Charm()
			if err != nil {
				return nil, errors.Trace(err)
			}
			if !ep.ImplementedBy(ch) {
				return nil, errors.Errorf("%q does not implement %q", ep.ServiceName, ep)
			}
			ops = append(ops, txn.Op{
				C:      servicesC,
				Id:     st.docID(ep.ServiceName),
				Assert: bson.D{{"life", Alive}, {"charmurl", ch.URL()}},
				Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
			})
		}
		if matchSeries && len(series) != 1 {
			return nil, errors.Errorf("principal and subordinate services' series must match")
		}
		// Create a new unique id if that has not already been done, and add
		// an operation to create the relation document.
		if id == -1 {
			var err error
			if id, err = st.sequence("relation"); err != nil {
				return nil, errors.Trace(err)
			}
		}
		doc = &relationDoc{
			Key:       key,
			Id:        id,
			Endpoints: eps,
			Life:      Alive,
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     doc.Key,
			Assert: txn.DocMissing,
			Insert: doc,
		})
		return ops, nil
	}
	if err = st.run(buildTxn); err == nil {
		return &Relation{st, *doc}, nil
	}
	return nil, errors.Trace(err)
}

// EndpointsRelation returns the existing relation with the given endpoints.
func (st *State) EndpointsRelation(endpoints ...Endpoint) (*Relation, error) {
	return st.KeyRelation(relationKey(endpoints))
}

// KeyRelation returns the existing relation with the given key (which can
// be derived unambiguously from the relation's endpoints).
func (st *State) KeyRelation(key string) (*Relation, error) {
	relations, closer := st.getCollection(relationsC)
	defer closer()

	doc := relationDoc{}
	err := relations.Find(bson.D{{"_id", key}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("relation %q", key)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get relation %q", key)
	}
	return newRelation(st, &doc), nil
}

// Relation returns the existing relation with the given id.
func (st *State) Relation(id int) (*Relation, error) {
	relations, closer := st.getCollection(relationsC)
	defer closer()

	doc := relationDoc{}
	err := relations.Find(bson.D{{"id", id}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("relation %d", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get relation %d", id)
	}
	return newRelation(st, &doc), nil
}

// AllRelations returns all relations in the environment ordered by id.
func (st *State) AllRelations() (relations []*Relation, err error) {
	relationsCollection, closer := st.getCollection(relationsC)
	defer closer()

	docs := relationDocSlice{}
	err = relationsCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get all relations")
	}
	sort.Sort(docs)
	for _, v := range docs {
		relations = append(relations, newRelation(st, &v))
	}
	return
}

type relationDocSlice []relationDoc

func (rdc relationDocSlice) Len() int      { return len(rdc) }
func (rdc relationDocSlice) Swap(i, j int) { rdc[i], rdc[j] = rdc[j], rdc[i] }
func (rdc relationDocSlice) Less(i, j int) bool {
	return rdc[i].Id < rdc[j].Id
}

// Action returns an Action by Id.
func (st *State) Action(id string) (*Action, error) {
	actions, closer := st.getCollection(actionsC)
	defer closer()

	doc := actionDoc{}
	err := actions.FindId(st.docID(id)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("action %q", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get action %q", id)
	}
	return newAction(st, doc), nil
}

// matchingActions finds actions that match ActionReceiver
func (st *State) matchingActions(ar ActionReceiver) ([]*Action, error) {
	return st.matchingActionsByReceiverName(ar.Name())
}

// matchingActionsByReceiverName returns all Actions associated with the
// ActionReceiver with the given name.
func (st *State) matchingActionsByReceiverName(receiver string) ([]*Action, error) {
	var doc actionDoc
	var actions []*Action

	actionsCollection, closer := st.getCollection(actionsC)
	defer closer()

	envuuid := st.EnvironTag().Id()
	sel := bson.D{{"env-uuid", envuuid}, {"receiver", receiver}}
	iter := actionsCollection.Find(sel).Iter()

	for iter.Next(&doc) {
		actions = append(actions, newAction(st, doc))
	}
	return actions, errors.Trace(iter.Close())

}

// ActionByTag returns an Action given an ActionTag
func (st *State) ActionByTag(tag names.ActionTag) (*Action, error) {
	return st.Action(actionIdFromTag(tag))
}

// ActionResult returns an ActionResult by Id.
func (st *State) ActionResult(id string) (*ActionResult, error) {
	actionresults, closer := st.getCollection(actionresultsC)
	defer closer()

	doc := actionResultDoc{}
	err := actionresults.FindId(st.docID(id)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("action result %q", id)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get actionresult %q", id)
	}
	return newActionResult(st, doc), nil
}

// matchingActionResults finds action results from a given ActionReceiver.
func (st *State) matchingActionResults(ar ActionReceiver) ([]*ActionResult, error) {
	var doc actionResultDoc
	var results []*ActionResult

	actionresults, closer := st.getCollection(actionresultsC)
	defer closer()

	envuuid := st.EnvironTag().Id()
	sel := bson.D{{"env-uuid", envuuid}, {"receiver", ar.Name()}}
	iter := actionresults.Find(sel).Iter()
	for iter.Next(&doc) {
		results = append(results, newActionResult(st, doc))
	}
	return results, errors.Trace(iter.Close())
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	if !names.IsValidUnit(name) {
		return nil, errors.Errorf("%q is not a valid unit name", name)
	}
	units, closer := st.getCollection(unitsC)
	defer closer()

	doc := unitDoc{}
	err := units.FindId(st.docID(name)).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("unit %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get unit %q", name)
	}
	return newUnit(st, &doc), nil
}

// AssignUnit places the unit on a machine. Depending on the policy, and the
// state of the environment, this may lead to new instances being launched
// within the environment.
func (st *State) AssignUnit(u *Unit, policy AssignmentPolicy) (err error) {
	if !u.IsPrincipal() {
		return errors.Errorf("subordinate unit %q cannot be assigned directly to a machine", u)
	}
	defer errors.Maskf(&err, "cannot assign unit %q to machine", u)
	var m *Machine
	switch policy {
	case AssignLocal:
		m, err = st.Machine("0")
		if err != nil {
			return errors.Trace(err)
		}
		return u.AssignToMachine(m)
	case AssignClean:
		if _, err = u.AssignToCleanMachine(); err != noCleanMachines {
			return errors.Trace(err)
		}
		return u.AssignToNewMachineOrContainer()
	case AssignCleanEmpty:
		if _, err = u.AssignToCleanEmptyMachine(); err != noCleanMachines {
			return errors.Trace(err)
		}
		return u.AssignToNewMachineOrContainer()
	case AssignNew:
		return errors.Trace(u.AssignToNewMachine())
	}
	return errors.Errorf("unknown unit assignment policy: %q", policy)
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
	err := mongo.SetAdminMongoPassword(st.db.Session, mongo.AdminUser, password)
	return errors.Trace(err)
}

type stateServersDoc struct {
	Id               string `bson:"_id"`
	EnvUUID          string `bson:"env-uuid"`
	MachineIds       []string
	VotingMachineIds []string
}

// StateServerInfo holds information about currently
// configured state server machines.
type StateServerInfo struct {
	// EnvironmentTag identifies the initial environment. Only the initial
	// environment is able to have machines that manage state. The initial
	// environment is the environment that is created when bootstrapping.
	EnvironmentTag names.EnvironTag

	// MachineIds holds the ids of all machines configured
	// to run a state server. It includes all the machine
	// ids in VotingMachineIds.
	MachineIds []string

	// VotingMachineIds holds the ids of all machines
	// configured to run a state server and to have a vote
	// in peer election.
	VotingMachineIds []string
}

// StateServerInfo returns information about
// the currently configured state server machines.
func (st *State) StateServerInfo() (*StateServerInfo, error) {
	stateServers, closer := st.getCollection(stateServersC)
	defer closer()

	var doc stateServersDoc
	err := stateServers.Find(bson.D{{"_id", environGlobalKey}}).One(&doc)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get state servers document")
	}

	if doc.EnvUUID == "" {
		logger.Warningf("state servers info has no environment UUID so retrieving it from environment")

		// This only happens when migrating from 1.20 to 1.21 before
		// upgrade steps have been run. Without this hack environTag
		// on State ends up empty, breaking basic functionality needed
		// to run upgrade steps (a chicken-and-egg scenario).
		environments, closer := st.getCollection(environmentsC)
		defer closer()

		var envDoc environmentDoc
		query := environments.Find(nil)
		count, err := query.Count()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get environment document count")
		}
		if count != 1 {
			return nil, errors.New("expected just one environment to get UUID from")
		}
		if err := query.One(&envDoc); err != nil {
			return nil, errors.Annotate(err, "cannot load environment document")
		}
		doc.EnvUUID = envDoc.UUID
	}

	return &StateServerInfo{
		EnvironmentTag:   names.NewEnvironTag(doc.EnvUUID),
		MachineIds:       doc.MachineIds,
		VotingMachineIds: doc.VotingMachineIds,
	}, nil
}

const stateServingInfoKey = "stateServingInfo"

// StateServingInfo returns information for running a state server machine
func (st *State) StateServingInfo() (StateServingInfo, error) {
	stateServers, closer := st.getCollection(stateServersC)
	defer closer()

	var info StateServingInfo
	err := stateServers.Find(bson.D{{"_id", stateServingInfoKey}}).One(&info)
	if err != nil {
		return info, errors.Trace(err)
	}
	if info.StatePort == 0 {
		return StateServingInfo{}, errors.NotFoundf("state serving info")
	}
	return info, nil
}

// SetStateServingInfo stores information needed for running a state server
func (st *State) SetStateServingInfo(info StateServingInfo) error {
	if info.StatePort == 0 || info.APIPort == 0 ||
		info.Cert == "" || info.PrivateKey == "" {
		return errors.Errorf("incomplete state serving info set in state")
	}
	ops := []txn.Op{{
		C:      stateServersC,
		Id:     stateServingInfoKey,
		Update: bson.D{{"$set", info}},
	}}
	if err := st.runTransaction(ops); err != nil {
		return errors.Annotatef(err, "cannot set state serving info")
	}
	return nil
}

var tagPrefix = map[byte]string{
	'm': names.MachineTagKind + "-",
	's': names.ServiceTagKind + "-",
	'u': names.UnitTagKind + "-",
	'e': names.EnvironTagKind + "-",
	'r': names.RelationTagKind + "-",
	'n': names.NetworkTagKind + "-",
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
