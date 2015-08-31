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
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"gopkg.in/juju/charm.v5"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/leadership"
	"github.com/juju/juju/state/lease"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.state")

const (
	// jujuDB is the name of the main juju database.
	jujuDB = "juju"

	// presenceDB is the name of the database used to hold presence pinger data.
	presenceDB = "presence"
	presenceC  = "presence"

	// blobstoreDB is the name of the blobstore GridFS database.
	blobstoreDB = "blobstore"

	// serviceLeadershipNamespace is the name of the lease.Client namespace
	// used by the leadership manager.
	serviceLeadershipNamespace = "service-leadership"
)

// State represents the state of an environment
// managed by juju.
type State struct {
	environTag names.EnvironTag
	serverTag  names.EnvironTag
	mongoInfo  *mongo.MongoInfo
	session    *mgo.Session
	database   Database
	policy     Policy

	// TODO(fwereade): move these out of state and make them independent
	// workers on which state depends.
	watcher           *watcher.Watcher
	pwatcher          *presence.Watcher
	leadershipManager leadership.ManagerWorker

	// mu guards allManager, allEnvManager & allEnvWatcherBacking
	mu                   sync.Mutex
	allManager           *storeManager
	allEnvManager        *storeManager
	allEnvWatcherBacking Backing

	// TODO(anastasiamac 2015-07-16) As state gets broken up, remove this.
	CloudImageMetadataStorage cloudimagemetadata.Storage
}

// StateServingInfo holds information needed by a state server.
// This type is a copy of the type of the same name from the api/params package.
// It is replicated here to avoid the state pacakge depending on api/params.
type StateServingInfo struct {
	APIPort      int
	StatePort    int
	Cert         string
	PrivateKey   string
	CAPrivateKey string
	// this will be passed as the KeyFile argument to MongoDB
	SharedSecret   string
	SystemIdentity string
}

// IsStateServer returns true if this state instance has the bootstrap
// environment UUID.
func (st *State) IsStateServer() bool {
	return st.environTag == st.serverTag
}

// RemoveAllEnvironDocs removes all documents from multi-environment
// collections. The environment should be put into a dying state before call
// this method. Otherwise, there is a race condition in which collections
// could be added to during or after the running of this method.
func (st *State) RemoveAllEnvironDocs() error {
	env, err := st.Environment()
	if err != nil {
		return errors.Trace(err)
	}
	id := userEnvNameIndex(env.Owner().Username(), env.Name())
	ops := []txn.Op{{
		// Cleanup the owner:envName unique key.
		C:      userenvnameC,
		Id:     id,
		Remove: true,
	}, {
		C:      environmentsC,
		Id:     st.EnvironUUID(),
		Assert: bson.D{{"life", Dying}},
		Remove: true,
	}}

	// Add all per-environment docs to the txn.
	for name, info := range st.database.Schema() {
		if info.global {
			continue
		}
		coll, closer := st.getCollection(name)
		defer closer()

		var ids []bson.M
		err := coll.Find(nil).Select(bson.D{{"_id", 1}}).All(&ids)
		if err != nil {
			return errors.Trace(err)
		}
		for _, id := range ids {
			if info.rawAccess {
				if err := coll.Writeable().RemoveId(id["_id"]); err != nil {
					return errors.Trace(err)
				}
			} else {
				ops = append(ops, txn.Op{
					C:      name,
					Id:     id["_id"],
					Remove: true,
				})
			}
		}
	}

	return st.runTransaction(ops)
}

// ForEnviron returns a connection to mongo for the specified environment. The
// connection uses the same credentials and policy as the existing connection.
func (st *State) ForEnviron(env names.EnvironTag) (*State, error) {
	newState, err := open(env, st.mongoInfo, mongo.DefaultDialOpts(), st.policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := newState.start(st.serverTag); err != nil {
		return nil, errors.Trace(err)
	}
	return newState, nil
}

// start starts the presence watcher, leadership manager and images metadata storage,
// and fills in the serverTag field with the supplied value.
func (st *State) start(serverTag names.EnvironTag) error {
	st.serverTag = serverTag

	var clientId string
	if identity := st.mongoInfo.Tag; identity != nil {
		// TODO(fwereade): it feels a bit wrong to take this from MongoInfo -- I
		// think it's just coincidental that the mongodb user happens to map to
		// the machine that's executing the code -- but there doesn't seem to be
		// an accessible alternative.
		clientId = identity.String()
	} else {
		// If we're running state anonymously, we can still use the lease
		// manager; but we need to make sure we use a unique client ID, and
		// will thus not be very performant.
		logger.Infof("running state anonymously; using unique client id")
		uuid, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		clientId = fmt.Sprintf("anon-%s", uuid.String())
	}

	logger.Infof("creating lease client as %s", clientId)
	clock := GetClock()
	datastore := &environMongo{st}
	leaseClient, err := lease.NewClient(lease.ClientConfig{
		Id:         clientId,
		Namespace:  serviceLeadershipNamespace,
		Collection: leasesC,
		Mongo:      datastore,
		Clock:      clock,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create lease client")
	}
	logger.Infof("starting leadership manager")
	leadershipManager, err := leadership.NewManager(leadership.ManagerConfig{
		Client: leaseClient,
		Clock:  clock,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create leadership manager")
	}
	st.leadershipManager = leadershipManager

	logger.Infof("creating cloud image metadata storage")
	st.CloudImageMetadataStorage = cloudimagemetadata.NewStorage(st.EnvironUUID(), cloudimagemetadataC, datastore)

	logger.Infof("starting presence watcher")
	st.pwatcher = presence.NewWatcher(st.getPresence(), st.environTag)
	return nil
}

// EnvironTag() returns the environment tag for the environment controlled by
// this state instance.
func (st *State) EnvironTag() names.EnvironTag {
	return st.environTag
}

// EnvironUUID returns the environment UUID for the environment
// controlled by this state instance.
func (st *State) EnvironUUID() string {
	return st.environTag.Id()
}

// userEnvNameIndex returns a string to be used as a userenvnameC unique index.
func userEnvNameIndex(username, envName string) string {
	return strings.ToLower(username) + ":" + envName
}

// EnsureEnvironmentRemoved returns an error if any multi-environment
// documents for this environment are found. It is intended only to be used in
// tests and exported so it can be used in the tests of other packages.
func (st *State) EnsureEnvironmentRemoved() error {
	found := map[string]int{}
	var foundOrdered []string
	for name, info := range st.database.Schema() {
		if info.global {
			continue
		}
		coll, closer := st.getCollection(name)
		defer closer()
		n, err := coll.Find(nil).Count()
		if err != nil {
			return errors.Trace(err)
		}
		if n != 0 {
			found[name] = n
			foundOrdered = append(foundOrdered, name)
		}
	}

	if len(found) != 0 {
		errMessage := fmt.Sprintf("found documents for environment with uuid %s:", st.EnvironUUID())
		sort.Strings(foundOrdered)
		for _, name := range foundOrdered {
			number := found[name]
			errMessage += fmt.Sprintf(" %d %s doc,", number, name)
		}
		// Remove trailing comma.
		errMessage = errMessage[:len(errMessage)-1]
		return errors.New(errMessage)
	}
	return nil
}

// getPresence returns the presence m.
func (st *State) getPresence() *mgo.Collection {
	return st.session.DB(presenceDB).C(presenceC)
}

// newDB returns a database connection using a new session, along with
// a closer function for the session. This is useful where you need to work
// with various collections in a single session, so don't want to call
// getCollection multiple times.
func (st *State) newDB() (Database, func()) {
	return st.database.CopySession()
}

// Ping probes the state's database connection to ensure
// that it is still alive.
func (st *State) Ping() error {
	return st.session.Ping()
}

// MongoSession returns the underlying mongodb session
// used by the state. It is exposed so that external code
// can maintain the mongo replica set and should not
// otherwise be used.
func (st *State) MongoSession() *mgo.Session {
	return st.session
}

type closeFunc func()

func (st *State) Watch() *Multiwatcher {
	st.mu.Lock()
	if st.allManager == nil {
		st.allManager = newStoreManager(newAllWatcherStateBacking(st))
	}
	st.mu.Unlock()
	return NewMultiwatcher(st.allManager)
}

func (st *State) WatchAllEnvs() *Multiwatcher {
	st.mu.Lock()
	if st.allEnvManager == nil {
		st.allEnvWatcherBacking = newAllEnvWatcherStateBacking(st)
		st.allEnvManager = newStoreManager(st.allEnvWatcherBacking)
	}
	st.mu.Unlock()
	return NewMultiwatcher(st.allEnvManager)
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
		collection, closer := st.getCollection(name)
		defer closer()
		var doc struct {
			DocID string `bson:"_id"`
		}
		iter := collection.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
		for iter.Next(&doc) {
			localID, err := st.strictLocalID(doc.DocID)
			if err != nil {
				return errors.Trace(err)
			}
			switch name {
			case machinesC:
				agentTags = append(agentTags, names.NewMachineTag(localID).String())
			case unitsC:
				agentTags = append(agentTags, names.NewUnitTag(localID).String())
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

// SetEnvironAgentVersion changes the agent version for the environment to the
// given version, only if the environment is in a stable state (all agents are
// running the current version). If this is a hosted environment, newVersion
// cannot be higher than the state server version.
func (st *State) SetEnvironAgentVersion(newVersion version.Number) (err error) {
	if newVersion.Compare(version.Current.Number) > 0 && !st.IsStateServer() {
		return errors.Errorf("a hosted environment cannot have a higher version than the server environment: %s > %s",
			newVersion.String(),
			version.Current.Number,
		)
	}

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
				Id:     st.docID(environGlobalKey),
				Assert: bson.D{{"txn-revno", settings.txnRevno}},
				Update: bson.D{
					{"$set", bson.D{{"agent-version", newVersion.String()}}},
				},
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
	mdoc, err := st.getMachineDoc(id)
	if err != nil {
		return nil, err
	}
	return newMachine(st, mdoc), nil
}

func (st *State) getMachineDoc(id string) (*machineDoc, error) {
	machinesCollection, closer := st.getRawCollection(machinesC)
	defer closer()

	var err error
	mdoc := &machineDoc{}
	for _, tryId := range []string{st.docID(id), id} {
		err = machinesCollection.FindId(tryId).One(mdoc)
		if err != mgo.ErrNotFound {
			break
		}
	}
	switch err {
	case nil:
		// This is required to allow loading of machines before the
		// environment UUID migration has been applied to the machines
		// collection. Without this, a machine agent can't come up to
		// run the database migration.
		if mdoc.Id == "" {
			mdoc.Id = mdoc.DocID
		}
		return mdoc, nil
	case mgo.ErrNotFound:
		return nil, errors.NotFoundf("machine %s", id)
	default:
		return nil, errors.Annotatef(err, "cannot get machine %s", id)
	}
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
	case names.IPAddressTag:
		return st.IPAddressByTag(tag)
	case names.ActionTag:
		return st.ActionByTag(tag)
	case names.CharmTag:
		if url, err := charm.ParseURL(id); err != nil {
			logger.Warningf("Parsing charm URL %q failed: %v", id, err)
			return nil, errors.NotFoundf("could not find charm %q in state", id)
		} else {
			return st.Charm(url)
		}
	case names.VolumeTag:
		return st.Volume(tag)
	case names.FilesystemTag:
		return st.Filesystem(tag)
	default:
		return nil, errors.Errorf("unsupported tag %T", tag)
	}
}

// tagToCollectionAndId, given an entity tag, returns the collection name and id
// of the entity document.
func (st *State) tagToCollectionAndId(tag names.Tag) (string, interface{}, error) {
	if tag == nil {
		return "", nil, errors.Errorf("tag is nil")
	}
	coll := ""
	id := tag.Id()
	switch tag := tag.(type) {
	case names.MachineTag:
		coll = machinesC
		id = st.docID(id)
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
		id = st.docID(id)
	case names.EnvironTag:
		coll = environmentsC
	case names.NetworkTag:
		coll = networksC
		id = st.docID(id)
	case names.ActionTag:
		coll = actionsC
		id = tag.Id()
	case names.CharmTag:
		coll = charmsC
		id = tag.Id()
	default:
		return "", nil, errors.Errorf("%q is not a valid collection tag", tag)
	}
	return coll, id, nil
}

// AddCharm adds the ch charm with curl to the state.
// On success the newly added charm state is returned.
func (st *State) AddCharm(ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string) (stch *Charm, err error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	query := charms.FindId(curl.String()).Select(bson.D{{"placeholder", 1}})

	buildTxn := func(attempt int) ([]txn.Op, error) {
		var placeholderDoc struct {
			Placeholder bool `bson:"placeholder"`
		}
		if err := query.One(&placeholderDoc); err == mgo.ErrNotFound {
			return insertCharmOps(st, ch, curl, storagePath, bundleSha256)
		} else if err != nil {
			return nil, errors.Trace(err)
		} else if placeholderDoc.Placeholder {
			return updateCharmOps(st, ch, curl, storagePath, bundleSha256, stillPlaceholder)
		}
		return nil, errors.AlreadyExistsf("charm %q", curl)
	}
	if err = st.run(buildTxn); err == nil {
		return st.Charm(curl)
	}
	return nil, errors.Trace(err)
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
		{"_id", curl.String()},
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
	curlRegex := "^" + regexp.QuoteMeta(st.docID(noRevURL.String()))
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
	curlRegex := "^" + regexp.QuoteMeta(st.docID(noRevURL.String()))

	charms, closer := st.getCollection(charmsC)
	defer closer()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Find the highest revision of that charm in state.
		var docs []charmDoc
		query := bson.D{{"_id", bson.D{{"$regex", curlRegex}}}}
		err = charms.Find(query).Select(bson.D{{"_id", 1}, {"url", 1}}).All(&docs)
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
		return insertPendingCharmOps(st, chosenUrl)
	}
	if err = st.run(buildTxn); err == nil {
		return chosenUrl, nil
	}
	return nil, errors.Trace(err)
}

// PrepareStoreCharmUpload must be called before a charm store charm
// is uploaded to the provider storage in order to create a charm
// document in state. If a charm with the same URL is already in
// state, it will be returned as a *state.Charm (it can be still
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
		err := charms.FindId(curl.String()).One(&uploadedCharm)
		switch {
		case err == mgo.ErrNotFound:
			uploadedCharm = charmDoc{
				DocID:         st.docID(curl.String()),
				EnvUUID:       st.EnvironTag().Id(),
				URL:           curl,
				PendingUpload: true,
			}
			return insertAnyCharmOps(&uploadedCharm)
		case err != nil:
			return nil, errors.Trace(err)
		case uploadedCharm.Placeholder:
			// Update the fields of the document we're returning.
			uploadedCharm.PendingUpload = true
			uploadedCharm.Placeholder = false
			return convertPlaceholderCharmOps(uploadedCharm.DocID)
		default:
			// The charm exists and it's either uploaded or still
			// pending, but it's not a placeholder. In any case,
			// there's nothing to do.
			return nil, jujutxn.ErrNoOperations
		}
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
		deleteOps, err := deleteOldPlaceholderCharmsOps(st, charms, curl)
		if err != nil {
			return nil, errors.Trace(err)
		}
		insertOps, err := insertPlaceholderCharmOps(st, curl)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops := append(deleteOps, insertOps...)
		return ops, nil
	}
	return errors.Trace(st.run(buildTxn))
}

// UpdateUploadedCharm marks the given charm URL as uploaded and
// updates the rest of its data, returning it as *state.Charm.
func (st *State) UpdateUploadedCharm(ch charm.Charm, curl *charm.URL, storagePath, bundleSha256 string) (*Charm, error) {
	charms, closer := st.getCollection(charmsC)
	defer closer()

	doc := &charmDoc{}
	err := charms.FindId(curl.String()).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("charm %q", curl)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !doc.PendingUpload {
		return nil, errors.Trace(&ErrCharmAlreadyUploaded{curl})
	}

	ops, err := updateCharmOps(st, ch, curl, storagePath, bundleSha256, stillPending)
	if err != nil {
		return nil, errors.Trace(err)
	}
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
			DocID:     st.docID(relKey),
			Key:       relKey,
			EnvUUID:   st.EnvironUUID(),
			Id:        relId,
			Endpoints: eps,
			Life:      Alive,
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     relDoc.DocID,
			Assert: txn.DocMissing,
			Insert: relDoc,
		})
	}
	return ops, nil
}

// AddService creates a new service, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddService(
	name, owner string, ch *Charm, networks []string, storage map[string]StorageConstraints,
) (service *Service, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add service %q", name)
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
	if exists, err := isNotDead(st, servicesC, name); err != nil {
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
	if storage == nil {
		storage = make(map[string]StorageConstraints)
	}
	if err := addDefaultStorageConstraints(st, storage, ch.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateStorageConstraints(st, storage, ch.Meta()); err != nil {
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

	statusDoc := statusDoc{
		EnvUUID: st.EnvironUUID(),
		// TODO(fwereade): this violates the spec. Should be "waiting".
		// Implemented like this to be consistent with incorrect add-unit
		// behaviour.
		Status:     StatusUnknown,
		StatusInfo: MessageWaitForAgentInit,
		Updated:    time.Now().UnixNano(),
		// This exists to preserve questionable unit-aggregation behaviour
		// while we work out how to switch to an implementation that makes
		// sense. It is also set in AddMissingServiceStatuses.
		NeverSet: true,
	}

	ops := []txn.Op{
		env.assertAliveOp(),
		createConstraintsOp(st, svc.globalKey(), constraints.Value{}),
		// TODO(dimitern) 2014-04-04 bug #1302498
		// Once we can add networks independently of machine
		// provisioning, we should check the given networks are valid
		// and known before setting them.
		createRequestedNetworksOp(st, svc.globalKey(), networks),
		createStorageConstraintsOp(svc.globalKey(), storage),
		createSettingsOp(st, svc.settingsKey(), nil),
		addLeadershipSettingsOp(svc.Tag().Id()),
		createStatusOp(st, svc.globalKey(), statusDoc),
		{
			C:      settingsrefsC,
			Id:     st.docID(svc.settingsKey()),
			Assert: txn.DocMissing,
			Insert: settingsRefsDoc{
				RefCount: 1,
				EnvUUID:  st.EnvironUUID()},
		}, {
			C:      servicesC,
			Id:     serviceID,
			Assert: txn.DocMissing,
			Insert: svcDoc,
		},
	}
	// Collect peer relation addition operations.
	peerOps, err := st.addPeerRelationsOps(name, peers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, peerOps...)

	// At the last moment before inserting the service, prime status history.
	probablyUpdateStatusHistory(st, svc.globalKey(), statusDoc)

	if err := st.runTransaction(ops); err == txn.ErrAborted {
		if err := checkEnvLife(st); err != nil {
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

// AddIPAddress creates and returns a new IP address. It can return an
// error satisfying IsNotValid() or IsAlreadyExists() when the addr
// does not contain a valid IP, or when addr is already added.
func (st *State) AddIPAddress(addr network.Address, subnetID string) (*IPAddress, error) {
	return addIPAddress(st, addr, subnetID)
}

// IPAddress returns an existing IP address from the state.
func (st *State) IPAddress(value string) (*IPAddress, error) {
	return ipAddress(st, value)
}

// IPAddressByTag returns an existing IP address from the state
// identified by its tag.
func (st *State) IPAddressByTag(tag names.IPAddressTag) (*IPAddress, error) {
	return ipAddressByTag(st, tag)
}

// AllocatedIPAddresses returns all the allocated addresses for a machine
func (st *State) AllocatedIPAddresses(machineId string) ([]*IPAddress, error) {
	return fetchIPAddresses(st, bson.D{{"machineid", machineId}})
}

// DeadIPAddresses returns all IP addresses with a Life of Dead
func (st *State) DeadIPAddresses() ([]*IPAddress, error) {
	return fetchIPAddresses(st, isDeadDoc)
}

// AddSubnet creates and returns a new subnet
func (st *State) AddSubnet(args SubnetInfo) (subnet *Subnet, err error) {
	defer errors.DeferredAnnotatef(&err, "adding subnet %q", args.CIDR)

	subnetID := st.docID(args.CIDR)
	subDoc := subnetDoc{
		DocID:             subnetID,
		EnvUUID:           st.EnvironUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        args.ProviderId,
		AllocatableIPHigh: args.AllocatableIPHigh,
		AllocatableIPLow:  args.AllocatableIPLow,
		AvailabilityZone:  args.AvailabilityZone,
		SpaceName:         args.SpaceName,
	}
	subnet = &Subnet{doc: subDoc, st: st}
	err = subnet.Validate()
	if err != nil {
		return nil, err
	}
	ops := []txn.Op{
		assertEnvAliveOp(st.EnvironUUID()),
		{
			C:      subnetsC,
			Id:     subnetID,
			Assert: txn.DocMissing,
			Insert: subDoc,
		},
	}

	err = st.runTransaction(ops)
	switch err {
	case txn.ErrAborted:
		if err := checkEnvLife(st); err != nil {
			return nil, errors.Trace(err)
		}
		if _, err = st.Subnet(args.CIDR); err == nil {
			return nil, errors.AlreadyExistsf("subnet %q", args.CIDR)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
	case nil:
		// if the ProviderId was not unique adding the subnet can fail
		// without an error. Refreshing catches this
		err = subnet.Refresh()
		if err == nil {
			return subnet, nil
		}
		return nil, errors.Errorf("ProviderId %q not unique", args.ProviderId)
	}
	return nil, errors.Trace(err)
}

func (st *State) Subnet(cidr string) (*Subnet, error) {
	subnets, closer := st.getCollection(subnetsC)
	defer closer()

	doc := &subnetDoc{}
	err := subnets.FindId(cidr).One(doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("subnet %q", cidr)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get subnet %q", cidr)
	}
	return &Subnet{st, *doc}, nil
}

// AllSubnets returns all known subnets in the environment.
func (st *State) AllSubnets() (subnets []*Subnet, err error) {
	subnetsCollection, closer := st.getCollection(subnetsC)
	defer closer()

	docs := []subnetDoc{}
	err = subnetsCollection.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all subnets")
	}
	for _, doc := range docs {
		subnets = append(subnets, &Subnet{st, doc})
	}
	return subnets, nil
}

// AddNetwork creates a new network with the given params. If a
// network with the same name or provider id already exists in state,
// an error satisfying errors.IsAlreadyExists is returned.
func (st *State) AddNetwork(args NetworkInfo) (n *Network, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add network %q", args.Name)
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
	doc := st.newNetworkDoc(args)
	ops := []txn.Op{
		assertEnvAliveOp(st.EnvironUUID()),
		{
			C:      networksC,
			Id:     doc.DocID,
			Assert: txn.DocMissing,
			Insert: doc,
		},
	}
	err = st.runTransaction(ops)
	switch err {
	case txn.ErrAborted:
		if err := checkEnvLife(st); err != nil {
			return nil, errors.Trace(err)
		}
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
	err = services.FindId(name).One(sdoc)
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
	return ensureEnvUUID(st.EnvironUUID(), localID)
}

// localID returns the local id value by stripping
// off the environment uuid prefix if it is there.
func (st *State) localID(ID string) string {
	envUUID, localID, ok := splitDocID(ID)
	if !ok || envUUID != st.EnvironUUID() {
		return ID
	}
	return localID
}

// strictLocalID returns the local id value by removing the
// environment UUID prefix.
//
// If there is no prefix matching the State's environment, an error is
// returned.
func (st *State) strictLocalID(ID string) (string, error) {
	envUUID, localID, ok := splitDocID(ID)
	if !ok || envUUID != st.EnvironUUID() {
		return "", errors.Errorf("unexpected id: %#v", ID)
	}
	return localID, nil
}

// ensureEnvUUID returns an environment UUID prefixed document ID. The
// prefix is only added if it isn't already there.
func ensureEnvUUID(envUUID, id string) string {
	prefix := envUUID + ":"
	if strings.HasPrefix(id, prefix) {
		return id
	}
	return prefix + id
}

// splitDocID returns the 2 parts of environment UUID prefixed
// document ID. If the id is not in the expected format the final
// return value will be false.
func splitDocID(id string) (string, string, bool) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
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
				if ep1.CanRelateTo(ep2) && containerScopeOk(st, ep1, ep2) {
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

func containerScopeOk(st *State, ep1, ep2 Endpoint) bool {
	if ep1.Scope != charm.ScopeContainer && ep2.Scope != charm.ScopeContainer {
		return true
	}
	var subordinateCount int
	for _, ep := range []Endpoint{ep1, ep2} {
		svc, err := st.Service(ep.ServiceName)
		if err != nil {
			return false
		}
		if svc.doc.Subordinate {
			subordinateCount++
		}
	}
	return subordinateCount >= 1
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
	defer errors.DeferredAnnotatef(&err, "cannot add relation %q", key)
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
		if exists, err := isNotDead(st, relationsC, key); err != nil {
			return nil, errors.Trace(err)
		} else if exists {
			return nil, errors.Errorf("relation already exists")
		}
		// Collect per-service operations, checking sanity as we go.
		var ops []txn.Op
		var subordinateCount int
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
			if svc.doc.Subordinate {
				subordinateCount++
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
		if eps[0].Scope == charm.ScopeContainer && subordinateCount < 1 {
			return nil, errors.Errorf("container scoped relation requires at least one subordinate service")
		}

		// Create a new unique id if that has not already been done, and add
		// an operation to create the relation document.
		if id == -1 {
			var err error
			if id, err = st.sequence("relation"); err != nil {
				return nil, errors.Trace(err)
			}
		}
		docID := st.docID(key)
		doc = &relationDoc{
			DocID:     docID,
			Key:       key,
			EnvUUID:   st.EnvironUUID(),
			Id:        id,
			Endpoints: eps,
			Life:      Alive,
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     docID,
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
	err := relations.FindId(key).One(&doc)
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

// Unit returns a unit by name.
func (st *State) Unit(name string) (*Unit, error) {
	if !names.IsValidUnit(name) {
		return nil, errors.Errorf("%q is not a valid unit name", name)
	}
	units, closer := st.getCollection(unitsC)
	defer closer()

	doc := unitDoc{}
	err := units.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("unit %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get unit %q", name)
	}
	return newUnit(st, &doc), nil
}

// UnitsFor returns the units placed in the given machine id.
func (st *State) UnitsFor(machineId string) ([]*Unit, error) {
	if !names.IsValidMachine(machineId) {
		return nil, errors.Errorf("%q is not a valid machine id", machineId)
	}
	m := &Machine{
		st: st,
		doc: machineDoc{
			Id: machineId,
		},
	}
	return m.Units()
}

// AssignUnit places the unit on a machine. Depending on the policy, and the
// state of the environment, this may lead to new instances being launched
// within the environment.
func (st *State) AssignUnit(u *Unit, policy AssignmentPolicy) (err error) {
	if !u.IsPrincipal() {
		return errors.Errorf("subordinate unit %q cannot be assigned directly to a machine", u)
	}
	defer errors.DeferredAnnotatef(&err, "cannot assign unit %q to machine", u)
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
	err := mongo.SetAdminMongoPassword(st.session, mongo.AdminUser, password)
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
	session := st.session.Copy()
	defer session.Close()
	return readRawStateServerInfo(st.session)
}

// readRawStateServerInfo reads StateServerInfo direct from the supplied session,
// falling back to the bootstrap environment document to extract the UUID when
// required.
func readRawStateServerInfo(session *mgo.Session) (*StateServerInfo, error) {
	db := session.DB(jujuDB)
	stateServers := db.C(stateServersC)

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
		environments := db.C(environmentsC)

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
	if info.CAPrivateKey == "" {
		// No CA certificate key means we can't generate new state server
		// certificates when needed to add to the certificate SANs.
		// Older Juju deployments discard the key because no one realised
		// the certificate was flawed, so at best we can log a warning
		// until an upgrade process is written.
		logger.Warningf("state serving info has no CA certificate key")
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

// SetSystemIdentity sets the system identity value in the database
// if and only iff it is empty.
func SetSystemIdentity(st *State, identity string) error {
	ops := []txn.Op{{
		C:      stateServersC,
		Id:     stateServingInfoKey,
		Assert: bson.D{{"systemidentity", ""}},
		Update: bson.D{{"$set", bson.D{{"systemidentity", identity}}}},
	}}

	if err := st.runTransaction(ops); err != nil {
		return errors.Trace(err)
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
