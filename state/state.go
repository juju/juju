// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state enables reading, observing, and changing
// the state stored in MongoDB of a whole environment
// managed by juju.
package state

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/juju/charm"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environmentserver/authentication"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/api/params"
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
	presenceC          = "presence"
	cleanupsC          = "cleanups"
	annotationsC       = "annotations"
	statusesC          = "statuses"
	stateServersC      = "stateServers"
	openedPortsC       = "openedPorts"
	backupsC           = "backups"

	// These collections are used by the mgo transaction runner.
	txnLogC = "txns.log"
	txnsC   = "txns"

	AdminUser = "admin"
)

// State represents the state of an environment
// managed by juju.
type State struct {
	// transactionRunner is normally nil, which means that a new one
	// will be created for each operation, ensuring a fresh mgo.Session
	// is used. However, for tests, a value may be assigned and this will
	// be used instead of creating a new runnner each time.
	transactionRunner jujutxn.Runner
	authenticated     bool
	mongoInfo         *authentication.MongoInfo
	policy            Policy
	db                *mgo.Database
	watcher           *watcher.Watcher
	pwatcher          *presence.Watcher
	// mu guards allManager.
	mu         sync.Mutex
	allManager *multiwatcher.StoreManager
	environTag names.EnvironTag
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
	if st.authenticated {
		return mongo.CollectionFromName(st.db, coll)
	}
	return st.db.C(coll), emptycloser
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

func emptycloser() {}

// txnRunner returns a jujutxn.Runner instance.
// If a runner has been assigned to st, that instance is returned.
// Otherwise a new instance is created.
// If st has been authenticated by having it's database logged in,
// a new mgo.Session is used.
func (st *State) txnRunner() (_ jujutxn.Runner, closer func()) {
	closer = emptycloser
	if st.transactionRunner != nil {
		return st.transactionRunner, closer
	}
	// If not authenticated, just use the unaltered db and a no-op closer.
	runnerDb := st.db
	if st.authenticated {
		session := runnerDb.Session.Copy()
		runnerDb = runnerDb.With(session)
		closer = session.Close
	}
	return jujutxn.NewRunner(jujutxn.RunnerParams{Database: runnerDb}), closer
}

// runTransaction is a convenience method delegating to transactionRunner.
func (st *State) runTransaction(ops []txn.Op) error {
	runner, closer := st.txnRunner()
	defer closer()
	return runner.RunTransaction(ops)
}

// run is a convenience method delegating to transactionRunner.
func (st *State) run(transactions jujutxn.TransactionSource) error {
	runner, closer := st.txnRunner()
	defer closer()
	return runner.Run(transactions)
}

// ResumeTransactions resumes all pending transactions.
func (st *State) ResumeTransactions() error {
	runner, closer := st.txnRunner()
	defer closer()
	return runner.ResumeTransactions()
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
				agentTags = append(agentTags, names.NewUnitTag(doc.Id).String())
			}
		}
		if err := iter.Close(); err != nil {
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
func (st *State) SetEnvironAgentVersion(newVersion version.Number) (err error) {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		settings, err := readSettings(st, environGlobalKey)
		if err != nil {
			return nil, err
		}
		agentVersion, ok := settings.Get("agent-version")
		if !ok {
			return nil, fmt.Errorf("no agent version set in the environment")
		}
		currentVersion, ok := agentVersion.(string)
		if !ok {
			return nil, fmt.Errorf("invalid agent version format: expected string, got %v", agentVersion)
		}
		if newVersion.String() == currentVersion {
			// Nothing to do.
			return nil, jujutxn.ErrNoOperations
		}

		if err := st.checkCanUpgrade(currentVersion, newVersion.String()); err != nil {
			return nil, err
		}

		ops := []txn.Op{{
			C:      settingsC,
			Id:     environGlobalKey,
			Assert: bson.D{{"txn-revno", settings.txnRevno}},
			Update: bson.D{{"$set", bson.D{{"agent-version", newVersion.String()}}}},
		}}
		return ops, nil
	}
	if err = st.run(buildTxn); err == jujutxn.ErrExcessiveContention {
		err = errors.Annotate(err, "cannot set agent version")
	}
	return err
}

func (st *State) buildAndValidateEnvironConfig(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
	newConfig, err := oldConfig.Apply(updateAttrs)
	if err != nil {
		return nil, err
	}
	if len(removeAttrs) != 0 {
		newConfig, err = newConfig.Remove(removeAttrs)
		if err != nil {
			return nil, err
		}
	}
	if err := checkEnvironConfig(newConfig); err != nil {
		return nil, err
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
		return err
	}

	// Get the existing environment config from state.
	oldConfig, err := config.New(config.NoDefaults, settings.Map())
	if err != nil {
		return err
	}
	if additionalValidation != nil {
		err = additionalValidation(updateAttrs, removeAttrs, oldConfig)
		if err != nil {
			return err
		}
	}
	validCfg, err := st.buildAndValidateEnvironConfig(updateAttrs, removeAttrs, oldConfig)
	if err != nil {
		return err
	}

	validAttrs := validCfg.AllAttrs()
	for k := range oldConfig.AllAttrs() {
		if _, ok := validAttrs[k]; !ok {
			settings.Delete(k)
		}
	}
	settings.Update(validAttrs)
	_, err = settings.Write()
	return err
}

// EnvironConstraints returns the current environment constraints.
func (st *State) EnvironConstraints() (constraints.Value, error) {
	return readConstraints(st, environGlobalKey)
}

// SetEnvironConstraints replaces the current environment constraints.
func (st *State) SetEnvironConstraints(cons constraints.Value) error {
	unsupported, err := st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting environment constraints: unsupported constraints: %v", strings.Join(unsupported, ","))
	} else if err != nil {
		return err
	}
	return writeConstraints(st, environGlobalKey, cons)
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
	machinesCollection, closer := st.getCollection(machinesC)
	defer closer()

	mdocs := machineDocSlice{}
	err = machinesCollection.Find(nil).All(&mdocs)
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
	machinesCollection, closer := st.getCollection(machinesC)
	defer closer()

	mdoc := &machineDoc{}
	sel := bson.D{{"_id", id}}
	err := machinesCollection.Find(sel).One(mdoc)
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
// *User, *Service, *Environment, or *Action, depending
// on the tag.
func (st *State) FindEntity(tag string) (Entity, error) {
	t, err := names.ParseTag(tag)
	if err != nil {
		return nil, err
	}
	id := t.Id()
	switch t.(type) {
	case names.MachineTag:
		return st.Machine(id)
	case names.UnitTag:
		return st.Unit(id)
	case names.UserTag:
		return st.User(id)
	case names.ServiceTag:
		return st.Service(id)
	case names.EnvironTag:
		env, err := st.Environment()
		if err != nil {
			return nil, err
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
		return st.ActionByTag(t)
	default:
		return nil, errors.Errorf("unsupported tag tpe %T", t)
	}
}

// parseTag, given an entity tag, returns the collection name and id
// of the entity document.
func (st *State) parseTag(tag names.Tag) (string, string, error) {
	if tag == nil {
		return "", "", errors.Errorf("tag is nil")
	}
	coll := ""
	id := tag.Id()
	switch tag := tag.(type) {
	case names.MachineTag:
		coll = machinesC
	case names.ServiceTag:
		coll = servicesC
	case names.UnitTag:
		coll = unitsC
	case names.UserTag:
		coll = usersC
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
		return "", "", fmt.Errorf("%q is not a valid collection tag", tag)
	}
	return coll, id, nil
}

// AddCharm adds the ch charm with curl to the state. bundleURL must
// be set to a URL where the bundle for ch may be downloaded from. On
// success the newly added charm state is returned.
func (st *State) AddCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (stch *Charm, err error) {
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
			BundleURL:    bundleURL,
			BundleSha256: bundleSha256,
		}
		err = charms.Insert(cdoc)
		if err != nil {
			return nil, fmt.Errorf("cannot add charm %q: %v", curl, err)
		}
		return newCharm(st, cdoc)
	} else if err != nil {
		return nil, err
	}
	return st.updateCharmDoc(ch, curl, bundleURL, bundleSha256, stillPlaceholder)
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
		return nil, fmt.Errorf("cannot get charm %q: %v", curl, err)
	}
	if err := cdoc.Meta.Check(); err != nil {
		return nil, fmt.Errorf("malformed charm metadata found in state: %v", err)
	}
	return newCharm(st, cdoc)
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
		return nil, fmt.Errorf("cannot get charm %q: %v", curl, err)
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
	return newCharm(st, &latest)
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
		return nil, fmt.Errorf("expected charm URL with local schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, fmt.Errorf("expected charm URL with revision, got %q", curl)
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
			return nil, err
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
	return nil, err
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
		return nil, fmt.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return nil, fmt.Errorf("expected charm URL with revision, got %q", curl)
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
			return nil, err
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
		return newCharm(st, &uploadedCharm)
	}
	return nil, err
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
		return fmt.Errorf("expected charm URL with cs schema, got %q", curl)
	}
	if curl.Revision < 0 {
		return fmt.Errorf("expected charm URL with revision, got %q", curl)
	}
	charms, closer := st.getCollection(charmsC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// See if the charm already exists in state and exit early if that's the case.
		var doc charmDoc
		err := charms.Find(bson.D{{"_id", curl.String()}}).Select(bson.D{{"_id", 1}}).One(&doc)
		if err != nil && err != mgo.ErrNotFound {
			return nil, err
		}
		if err == nil {
			return nil, jujutxn.ErrNoOperations
		}

		// Delete all previous placeholders so we don't fill up the database with unused data.
		ops, err := st.deleteOldPlaceholderCharmsOps(curl)
		if err != nil {
			return nil, err
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
	return st.run(buildTxn)
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
		return nil, err
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
	_, ok := err.(*ErrCharmAlreadyUploaded)
	return ok
}

// ErrCharmRevisionAlreadyModified is returned when a pending or
// placeholder charm is no longer pending or a placeholder, signaling
// the charm is available in state with its full information.
var ErrCharmRevisionAlreadyModified = fmt.Errorf("charm revision already modified")

// UpdateUploadedCharm marks the given charm URL as uploaded and
// updates the rest of its data, returning it as *state.Charm.
func (st *State) UpdateUploadedCharm(ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	doc := &charmDoc{}
	err := charms.FindId(curl).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, err
	}
	if !doc.PendingUpload {
		return nil, &ErrCharmAlreadyUploaded{curl}
	}

	return st.updateCharmDoc(ch, curl, bundleURL, bundleSha256, stillPending)
}

// updateCharmDoc updates the charm with specified URL with the given
// data, and resets the placeholder and pendingupdate flags.  If the
// charm is no longer a placeholder or pending (depending on preReq),
// it returns ErrCharmRevisionAlreadyModified.
func (st *State) updateCharmDoc(
	ch charm.Charm, curl *charm.URL, bundleURL *url.URL, bundleSha256 string, preReq interface{}) (*Charm, error) {

	updateFields := bson.D{{"$set", bson.D{
		{"meta", ch.Meta()},
		{"config", ch.Config()},
		{"actions", ch.Actions()},
		{"bundleurl", bundleURL},
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
func (st *State) AddService(name, ownerTag string, ch *Charm, networks []string) (service *Service, err error) {
	defer errors.Maskf(&err, "cannot add service %q", name)
	tag, err := names.ParseUserTag(ownerTag)
	if err != nil {
		return nil, fmt.Errorf("Invalid ownertag %s: %v", ownerTag, err)
	}
	// Sanity checks.
	if !names.IsValidService(name) {
		return nil, fmt.Errorf("invalid name")
	}
	if ch == nil {
		return nil, fmt.Errorf("charm is nil")
	}
	if exists, err := isNotDead(st.db, servicesC, name); err != nil {
		return nil, err
	} else if exists {
		return nil, fmt.Errorf("service already exists")
	}
	env, err := st.Environment()
	if err != nil {
		return nil, err
	} else if env.Life() != Alive {
		return nil, fmt.Errorf("environment is no longer alive")
	}
	ownerId := tag.Id()
	if userExists, err := st.checkUserExists(ownerId); err != nil {
		return nil, err
	} else if !userExists {
		return nil, fmt.Errorf("user %v doesn't exist", ownerId)
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
		OwnerTag:      ownerTag,
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
			C:      usersC,
			Id:     ownerId,
			Assert: txn.DocExists,
		},
		{
			C:      settingsrefsC,
			Id:     svc.settingsKey(),
			Assert: txn.DocMissing,
			Insert: settingsRefsDoc{1},
		},
		{
			C:      servicesC,
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

	if err := st.runTransaction(ops); err == txn.ErrAborted {
		err := env.Refresh()
		if (err == nil && env.Life() != Alive) || errors.IsNotFound(err) {
			return nil, fmt.Errorf("environment is no longer alive")
		} else if err != nil {
			return nil, err
		}

		if userExists, ueErr := st.checkUserExists(ownerId); ueErr != nil {
			return nil, ueErr
		} else if !userExists {
			return nil, fmt.Errorf("unknown user %q", ownerId)
		}

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
		return nil, fmt.Errorf("name must be not empty")
	}
	if !names.IsValidNetwork(args.Name) {
		return nil, fmt.Errorf("invalid name")
	}
	if args.ProviderId == "" {
		return nil, fmt.Errorf("provider id must be not empty")
	}
	if args.VLANTag < 0 || args.VLANTag > 4094 {
		return nil, fmt.Errorf("invalid VLAN tag %d: must be between 0 and 4094", args.VLANTag)
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
			return nil, err
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
	return nil, err
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
		return nil, fmt.Errorf("cannot get network %q: %v", name, err)
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
		return nil, fmt.Errorf("cannot get all networks")
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
		return nil, fmt.Errorf("%q is not a valid service name", name)
	}
	sdoc := &serviceDoc{}
	sel := bson.D{{"_id", name}}
	err = services.Find(sel).One(sdoc)
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
	servicesCollection, closer := st.getCollection(servicesC)
	defer closer()

	sdocs := []serviceDoc{}
	err = servicesCollection.Find(bson.D{}).All(&sdocs)
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
	defer errors.Maskf(&err, "cannot add relation %q", key)
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
	// we'll need to re-validate service sanity.
	var doc *relationDoc
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Perform initial relation sanity check.
		if exists, err := isNotDead(st.db, relationsC, key); err != nil {
			return nil, err
		} else if exists {
			return nil, fmt.Errorf("relation already exists")
		}
		// Collect per-service operations, checking sanity as we go.
		var ops []txn.Op
		series := map[string]bool{}
		for _, ep := range eps {
			svc, err := st.Service(ep.ServiceName)
			if errors.IsNotFound(err) {
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
				C:      servicesC,
				Id:     ep.ServiceName,
				Assert: bson.D{{"life", Alive}, {"charmurl", ch.URL()}},
				Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
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
	return nil, err
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
		return nil, fmt.Errorf("cannot get relation %q: %v", key, err)
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
		return nil, fmt.Errorf("cannot get relation %d: %v", id, err)
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
	err := actions.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("action %q", id)
	}
	if err != nil {
		return nil, errors.Errorf("cannot get action %q: %v", id, err)
	}
	return newAction(st, doc), nil
}

// ActionByTag returns an Action given an ActionTag
func (st *State) ActionByTag(tag names.Tag) (*Action, error) {
	actionTag, ok := tag.(names.ActionTag)
	if !ok {
		return nil, fmt.Errorf("cannot get action from tag %v", tag)
	}
	return st.Action(actionIdFromTag(actionTag))
}

// matchingActions finds actions that match ActionReceiver
func (st *State) matchingActions(ar ActionReceiver) ([]*Action, error) {
	return st.matchingActionsByPrefix(ar.Name())
}

// matchingActionsByPrefix finds actions with a given prefix
func (st *State) matchingActionsByPrefix(prefix string) ([]*Action, error) {
	var doc actionDoc
	var actions []*Action

	actionsCollection, closer := st.getCollection(actionsC)
	defer closer()

	sel := bson.D{{"_id", bson.D{{"$regex", "^" + regexp.QuoteMeta(ensureActionMarker(prefix))}}}}
	iter := actionsCollection.Find(sel).Iter()

	for iter.Next(&doc) {
		actions = append(actions, newAction(st, doc))
	}
	return actions, iter.Close()
}

// ActionResult returns an ActionResult by Id.
func (st *State) ActionResult(id string) (*ActionResult, error) {
	actionresults, closer := st.getCollection(actionresultsC)
	defer closer()

	doc := actionResultDoc{}
	err := actionresults.FindId(id).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("action result %q", id)
	}
	if err != nil {
		return nil, errors.Errorf("cannot get actionresult %q: %v", id, err)
	}
	return newActionResult(st, doc), nil
}

// matchingActionResults finds actions that match name
func (st *State) matchingActionResults(ar ActionReceiver) ([]*ActionResult, error) {
	var doc actionResultDoc
	var results []*ActionResult

	actionresults, closer := st.getCollection(actionresultsC)
	defer closer()

	prefix := actionResultPrefix(ar)
	sel := bson.D{{"_id", bson.D{{"$regex", "^" + regexp.QuoteMeta(prefix)}}}}
	iter := actionresults.Find(sel).Iter()
	for iter.Next(&doc) {
		results = append(results, newActionResult(st, doc))
	}
	return results, iter.Close()
}

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	if !names.IsValidUnit(name) {
		return nil, fmt.Errorf("%q is not a valid unit name", name)
	}
	units, closer := st.getCollection(unitsC)
	defer closer()

	doc := unitDoc{}
	err := units.FindId(name).One(&doc)
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
	defer errors.Maskf(&err, "cannot assign unit %q to machine", u)
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
	return mongo.SetAdminMongoPassword(st.db.Session, AdminUser, password)
}

func (st *State) setMongoPassword(name, password string) error {
	return mongo.SetMongoPassword(name, password,
		st.db,
		st.db.Session.DB("presence"),
		st.db.Session.DB("admin"))
}

type stateServersDoc struct {
	Id               string `bson:"_id"`
	MachineIds       []string
	VotingMachineIds []string
}

// StateServerInfo holds information about currently
// configured state server machines.
type StateServerInfo struct {
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
		return nil, fmt.Errorf("cannot get state servers document: %v", err)
	}
	return &StateServerInfo{
		MachineIds:       doc.MachineIds,
		VotingMachineIds: doc.VotingMachineIds,
	}, nil
}

const stateServingInfoKey = "stateServingInfo"

// StateServingInfo returns information for running a state server machine
func (st *State) StateServingInfo() (params.StateServingInfo, error) {
	stateServers, closer := st.getCollection(stateServersC)
	defer closer()

	var info params.StateServingInfo
	err := stateServers.Find(bson.D{{"_id", stateServingInfoKey}}).One(&info)
	if err != nil {
		return info, err
	}
	if info.StatePort == 0 {
		return params.StateServingInfo{}, errors.NotFoundf("state serving info")
	}
	return info, nil
}

// SetStateServingInfo stores information needed for running a state server
func (st *State) SetStateServingInfo(info params.StateServingInfo) error {
	if info.StatePort == 0 || info.APIPort == 0 ||
		info.Cert == "" || info.PrivateKey == "" {
		return fmt.Errorf("incomplete state serving info set in state")
	}
	ops := []txn.Op{{
		C:      stateServersC,
		Id:     stateServingInfoKey,
		Update: bson.D{{"$set", info}},
	}}
	if err := st.runTransaction(ops); err != nil {
		return fmt.Errorf("cannot set state serving info: %v", err)
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
