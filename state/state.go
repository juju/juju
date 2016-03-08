// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state enables reading, observing, and changing
// the state stored in MongoDB of a whole model
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
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/cloudimagemetadata"
	statelease "github.com/juju/juju/state/lease"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/lease"
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

	// singularControllerNamespace is the name of the lease.Client namespace
	// used by the singular manager
	singularControllerNamespace = "singular-controller"
)

// State represents the state of an model
// managed by juju.
type State struct {
	modelTag      names.ModelTag
	controllerTag names.ModelTag
	mongoInfo     *mongo.MongoInfo
	session       *mgo.Session
	database      Database
	policy        Policy

	// TODO(fwereade): move these out of state and make them independent
	// workers on which state depends.
	watcher  *watcher.Watcher
	pwatcher *presence.Watcher
	// leadershipManager keeps track of units' service leadership leases
	// within this environment.
	leadershipManager *lease.Manager
	// singularManager keeps track of which controller machine is responsible
	// for managing this state's environment.
	singularManager *lease.Manager

	// mu guards allManager, allModelManager & allModelWatcherBacking
	mu                     sync.Mutex
	allManager             *storeManager
	allModelManager        *storeManager
	allModelWatcherBacking Backing

	// TODO(anastasiamac 2015-07-16) As state gets broken up, remove this.
	CloudImageMetadataStorage cloudimagemetadata.Storage
}

// StateServingInfo holds information needed by a controller.
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

// IsController returns true if this state instance has the bootstrap
// model UUID.
func (st *State) IsController() bool {
	return st.modelTag == st.controllerTag
}

// RemoveAllModelDocs removes all documents from multi-model
// collections. The model should be put into a dying state before call
// this method. Otherwise, there is a race condition in which collections
// could be added to during or after the running of this method.
func (st *State) RemoveAllModelDocs() error {
	env, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	id := userModelNameIndex(env.Owner().Canonical(), env.Name())
	ops := []txn.Op{{
		// Cleanup the owner:envName unique key.
		C:      usermodelnameC,
		Id:     id,
		Remove: true,
	}, {
		C:      modelsC,
		Id:     st.ModelUUID(),
		Assert: bson.D{{"life", Dead}},
		Remove: true,
	}}

	// Add all per-model docs to the txn.
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

// ForModel returns a connection to mongo for the specified model. The
// connection uses the same credentials and policy as the existing connection.
func (st *State) ForModel(env names.ModelTag) (*State, error) {
	newState, err := open(env, st.mongoInfo, mongo.DefaultDialOpts(), st.policy)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := newState.start(st.controllerTag); err != nil {
		return nil, errors.Trace(err)
	}
	return newState, nil
}

// start starts the presence watcher, leadership manager and images metadata storage,
// and fills in the controllerTag field with the supplied value.
func (st *State) start(controllerTag names.ModelTag) error {
	st.controllerTag = controllerTag

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

	logger.Infof("creating lease clients as %s", clientId)
	clock := GetClock()
	datastore := &environMongo{st}
	leadershipClient, err := statelease.NewClient(statelease.ClientConfig{
		Id:         clientId,
		Namespace:  serviceLeadershipNamespace,
		Collection: leasesC,
		Mongo:      datastore,
		Clock:      clock,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create leadership lease client")
	}
	logger.Infof("starting leadership lease manager")
	leadershipManager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: leadershipSecretary{},
		Client:    leadershipClient,
		Clock:     clock,
		MaxSleep:  time.Minute,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create leadership lease manager")
	}
	st.leadershipManager = leadershipManager

	singularClient, err := statelease.NewClient(statelease.ClientConfig{
		Id:         clientId,
		Namespace:  singularControllerNamespace,
		Collection: leasesC,
		Mongo:      datastore,
		Clock:      clock,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create singular lease client")
	}
	logger.Infof("starting singular lease manager")
	singularManager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: singularSecretary{st.modelTag.Id()},
		Client:    singularClient,
		Clock:     clock,
		MaxSleep:  time.Minute,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create singular lease manager")
	}
	st.singularManager = singularManager

	logger.Infof("creating cloud image metadata storage")
	st.CloudImageMetadataStorage = cloudimagemetadata.NewStorage(st.ModelUUID(), cloudimagemetadataC, datastore)

	logger.Infof("starting presence watcher")
	st.pwatcher = presence.NewWatcher(st.getPresence(), st.modelTag)
	return nil
}

// ModelTag() returns the model tag for the model controlled by
// this state instance.
func (st *State) ModelTag() names.ModelTag {
	return st.modelTag
}

// ModelUUID returns the model UUID for the model
// controlled by this state instance.
func (st *State) ModelUUID() string {
	return st.modelTag.Id()
}

// userModelNameIndex returns a string to be used as a usermodelnameC unique index.
func userModelNameIndex(username, envName string) string {
	return strings.ToLower(username) + ":" + envName
}

// EnsureModelRemoved returns an error if any multi-model
// documents for this model are found. It is intended only to be used in
// tests and exported so it can be used in the tests of other packages.
func (st *State) EnsureModelRemoved() error {
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
		errMessage := fmt.Sprintf("found documents for model with uuid %s:", st.ModelUUID())
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

// MongoVersion return the string repre
func (st *State) MongoVersion() (string, error) {
	binfo, err := st.session.BuildInfo()
	if err != nil {
		return "", errors.Annotate(err, "cannot obtain mongo build info")
	}
	return binfo.Version, nil
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

func (st *State) WatchAllModels() *Multiwatcher {
	st.mu.Lock()
	if st.allModelManager == nil {
		st.allModelWatcherBacking = NewAllModelWatcherStateBacking(st)
		st.allModelManager = newStoreManager(st.allModelWatcherBacking)
	}
	st.mu.Unlock()
	return NewMultiwatcher(st.allModelManager)
}

func (st *State) ModelConfig() (*config.Config, error) {
	settings, err := readSettings(st, modelGlobalKey)
	if err != nil {
		return nil, errors.Trace(err)
	}
	attrs := settings.Map()
	return config.New(config.NoDefaults, attrs)
}

// checkModelConfig returns an error if the config is definitely invalid.
func checkModelConfig(cfg *config.Config) error {
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
	return fmt.Sprintf("some agents have not upgraded to the current model version %s: %s", e.currentVersion, strings.Join(e.agents, ", "))
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

var errUpgradeInProgress = errors.New(params.CodeUpgradeInProgress)

// IsUpgradeInProgressError returns true if the error is cause by an
// upgrade in progress
func IsUpgradeInProgressError(err error) bool {
	return errors.Cause(err) == errUpgradeInProgress
}

// SetModelAgentVersion changes the agent version for the model to the
// given version, only if the model is in a stable state (all agents are
// running the current version). If this is a hosted model, newVersion
// cannot be higher than the controller version.
func (st *State) SetModelAgentVersion(newVersion version.Number) (err error) {
	if newVersion.Compare(version.Current) > 0 && !st.IsController() {
		return errors.Errorf("a hosted model cannot have a higher version than the server model: %s > %s",
			newVersion.String(),
			version.Current,
		)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		settings, err := readSettings(st, modelGlobalKey)
		if err != nil {
			return nil, errors.Trace(err)
		}
		agentVersion, ok := settings.Get("agent-version")
		if !ok {
			return nil, errors.Errorf("no agent version set in the model")
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
				Id:     st.docID(modelGlobalKey),
				Assert: bson.D{{"version", settings.version}},
				Update: bson.D{
					{"$set", bson.D{{"settings.agent-version", newVersion.String()}}},
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
			err = errUpgradeInProgress
		} else {
			err = errors.Annotate(err, "cannot set agent version")
		}
	}
	return errors.Trace(err)
}

func (st *State) buildAndValidateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) (validCfg *config.Config, err error) {
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
	if err := checkModelConfig(newConfig); err != nil {
		return nil, errors.Trace(err)
	}
	return st.validate(newConfig, oldConfig)
}

type ValidateConfigFunc func(updateAttrs map[string]interface{}, removeAttrs []string, oldConfig *config.Config) error

// UpdateModelConfig adds, updates or removes attributes in the current
// configuration of the model with the provided updateAttrs and
// removeAttrs.
func (st *State) UpdateModelConfig(updateAttrs map[string]interface{}, removeAttrs []string, additionalValidation ValidateConfigFunc) error {
	if len(updateAttrs)+len(removeAttrs) == 0 {
		return nil
	}

	// TODO(axw) 2013-12-6 #1167616
	// Ensure that the settings on disk have not changed
	// underneath us. The settings changes are actually
	// applied as a delta to what's on disk; if there has
	// been a concurrent update, the change may not be what
	// the user asked for.
	settings, err := readSettings(st, modelGlobalKey)
	if err != nil {
		return errors.Trace(err)
	}

	// Get the existing model config from state.
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
	validCfg, err := st.buildAndValidateModelConfig(updateAttrs, removeAttrs, oldConfig)
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

// EnvironConstraints returns the current model constraints.
func (st *State) ModelConstraints() (constraints.Value, error) {
	cons, err := readConstraints(st, modelGlobalKey)
	return cons, errors.Trace(err)
}

// SetEnvironConstraints replaces the current model constraints.
func (st *State) SetModelConstraints(cons constraints.Value) error {
	unsupported, err := st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"setting model constraints: unsupported constraints: %v", strings.Join(unsupported, ","))
	} else if err != nil {
		return errors.Trace(err)
	}
	return writeConstraints(st, modelGlobalKey, cons)
}

// AllMachines returns all machines in the model
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
		// model UUID migration has been applied to the machines
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
// *User, *Service, *Model, or *Action, depending
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
	case names.ModelTag:
		env, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Return an invalid entity error if the requested model is not
		// the current one.
		if id != env.UUID() {
			if utils.IsValidUUIDString(id) {
				return nil, errors.NotFoundf("model %q", id)
			}
			// TODO(axw) 2013-12-04 #1257587
			// We should not accept model tags that do not match the
			// model's UUID. We accept anything for now, to cater
			// both for past usage, and for potentially supporting aliases.
			logger.Warningf("model-tag does not match current model UUID: %q != %q", id, env.UUID())
			conf, err := st.ModelConfig()
			if err != nil {
				logger.Warningf("ModelConfig failed: %v", err)
			} else if id != conf.Name() {
				logger.Warningf("model-tag does not match current model name: %q != %q", id, conf.Name())
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
			return "", nil, fmt.Errorf("%q is not a local user", tag.Canonical())
		}
		id = tag.Name()
	case names.RelationTag:
		coll = relationsC
		id = st.docID(id)
	case names.ModelTag:
		coll = modelsC
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
				ModelUUID:     st.ModelTag().Id(),
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
			ModelUUID: st.ModelUUID(),
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

type AddServiceArgs struct {
	Name             string
	Series           string
	Owner            string
	Charm            *Charm
	Networks         []string
	Storage          map[string]StorageConstraints
	EndpointBindings map[string]string
	Settings         charm.Settings
	NumUnits         int
	Placement        []*instance.Placement
	Constraints      constraints.Value
	Resources        map[string]string
}

// AddService creates a new service, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddService(args AddServiceArgs) (service *Service, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add service %q", args.Name)
	ownerTag, err := names.ParseUserTag(args.Owner)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid ownertag %s", args.Owner)
	}
	// Sanity checks.
	if !names.IsValidService(args.Name) {
		return nil, errors.Errorf("invalid name")
	}
	if args.Charm == nil {
		return nil, errors.Errorf("charm is nil")
	}
	if exists, err := isNotDead(st, servicesC, args.Name); err != nil {
		return nil, errors.Trace(err)
	} else if exists {
		return nil, errors.Errorf("service already exists")
	}
	env, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	} else if env.Life() != Alive {
		return nil, errors.Errorf("model is no longer alive")
	}
	if _, err := st.ModelUser(ownerTag); err != nil {
		return nil, errors.Trace(err)
	}
	if args.Storage == nil {
		args.Storage = make(map[string]StorageConstraints)
	}
	if err := addDefaultStorageConstraints(st, args.Storage, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateStorageConstraints(st, args.Storage, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	storagePools := make(set.Strings)
	for _, storageParams := range args.Storage {
		storagePools.Add(storageParams.Pool)
	}

	if args.Series == "" {
		// args.Series is not set, so use the series in the URL.
		args.Series = args.Charm.URL().Series
		if args.Series == "" {
			// Should not happen, but just in case.
			return nil, errors.New("series is empty")
		}
	} else {
		// User has specified series. Overriding supported series is
		// handled by the client, so args.Series is not necessarily
		// one of the charm's supported series. We require that the
		// specified series is of the same operating system as one of
		// the supported series. For old-style charms with the series
		// in the URL, that series is the one and only supported
		// series.
		var supportedSeries []string
		if series := args.Charm.URL().Series; series != "" {
			supportedSeries = []string{series}
		} else {
			supportedSeries = args.Charm.Meta().Series
		}
		seriesOS, err := series.GetOSFromSeries(args.Series)
		if err != nil {
			return nil, errors.Trace(err)
		}
		supportedOperatingSystems := make(map[os.OSType]bool)
		for _, supportedSeries := range supportedSeries {
			os, err := series.GetOSFromSeries(supportedSeries)
			if err != nil {
				return nil, errors.Trace(err)
			}
			supportedOperatingSystems[os] = true
		}
		if !supportedOperatingSystems[seriesOS] {
			return nil, errors.NewNotSupported(errors.Errorf(
				"series %q (OS %q) not supported by charm",
				args.Series, seriesOS,
			), "")
		}
	}

	for _, placement := range args.Placement {
		data, err := st.parsePlacement(placement)
		if err != nil {
			return nil, errors.Trace(err)
		}
		switch data.placementType() {
		case machinePlacement:
			// Ensure that the machine and charm series match.
			m, err := st.Machine(data.machineId)
			if err != nil {
				return nil, errors.Trace(err)
			}
			subordinate := args.Charm.Meta().Subordinate
			if err := validateUnitMachineAssignment(
				m, args.Series, subordinate, storagePools,
			); err != nil {
				return nil, errors.Annotatef(
					err, "cannot deploy to machine %s", m,
				)
			}

		case directivePlacement:
			if err := st.precheckInstance(args.Series, args.Constraints, data.directive); err != nil {
				return nil, errors.Trace(err)
			}
		}
	}

	serviceID := st.docID(args.Name)

	// Create the service addition operations.
	peers := args.Charm.Meta().Peers

	// The doc defaults to CharmModifiedVersion = 0, which is correct, since it
	// has, by definition, at its initial state.
	svcDoc := &serviceDoc{
		DocID:         serviceID,
		Name:          args.Name,
		ModelUUID:     env.UUID(),
		Series:        args.Series,
		Subordinate:   args.Charm.Meta().Subordinate,
		CharmURL:      args.Charm.URL(),
		RelationCount: len(peers),
		Life:          Alive,
		OwnerTag:      args.Owner,
	}

	svc := newService(st, svcDoc)

	endpointBindingsOp, err := createEndpointBindingsOp(
		st, svc.globalKey(),
		args.EndpointBindings, args.Charm.Meta(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	statusDoc := statusDoc{
		ModelUUID: st.ModelUUID(),
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

	// The addServiceOps does not include the environment alive assertion,
	// so we add it here.
	ops := append(
		[]txn.Op{
			env.assertAliveOp(),
			endpointBindingsOp,
		},
		addServiceOps(st, addServiceOpsArgs{
			serviceDoc:       svcDoc,
			statusDoc:        statusDoc,
			constraints:      args.Constraints,
			storage:          args.Storage,
			settings:         map[string]interface{}(args.Settings),
			settingsRefCount: 1,
		})...)

	// Collect peer relation addition operations.
	//
	// TODO(dimitern): Ensure each st.Endpoint has a space name associated in a
	// follow-up.
	peerOps, err := st.addPeerRelationsOps(args.Name, peers)
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, peerOps...)

	if len(args.Resources) > 0 {
		// Collect pending resource resolution operations.
		resources, err := st.Resources()
		if err != nil {
			return nil, errors.Trace(err)
		}
		resOps, err := resources.NewResolvePendingResourcesOps(args.Name, args.Resources)
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, resOps...)
	}

	// Collect unit-adding operations.
	for x := 0; x < args.NumUnits; x++ {
		unitName, unitOps, err := svc.addServiceUnitOps(serviceAddUnitOpsArgs{cons: args.Constraints, storageCons: args.Storage})
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, unitOps...)
		placement := instance.Placement{}
		if x < len(args.Placement) {
			placement = *args.Placement[x]
		}
		ops = append(ops, assignUnitOps(unitName, placement)...)
	}
	// At the last moment before inserting the service, prime status history.
	probablyUpdateStatusHistory(st, svc.globalKey(), statusDoc)

	if err := st.runTransaction(ops); err == txn.ErrAborted {
		if err := checkModeLife(st); err != nil {
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

// TODO(natefinch) DEMO code, revisit after demo!
var AddServicePostFuncs = map[string]func(*State, AddServiceArgs) error{}

// assignUnitOps returns the db ops to save unit assignment for use by the
// UnitAssigner worker.
func assignUnitOps(unitName string, placement instance.Placement) []txn.Op {
	udoc := assignUnitDoc{
		DocId:     unitName,
		Scope:     placement.Scope,
		Directive: placement.Directive,
	}
	return []txn.Op{{
		C:      assignUnitC,
		Id:     udoc.DocId,
		Assert: txn.DocMissing,
		Insert: udoc,
	}}
}

// AssignStagedUnits gets called by the UnitAssigner worker, and runs the given
// assignments.
func (st *State) AssignStagedUnits(ids []string) ([]UnitAssignmentResult, error) {
	query := bson.D{{"_id", bson.D{{"$in", ids}}}}
	unitAssignments, err := st.unitAssignments(query)
	if err != nil {
		return nil, errors.Annotate(err, "getting staged unit assignments")
	}
	results := make([]UnitAssignmentResult, len(unitAssignments))
	for i, a := range unitAssignments {
		err := st.assignStagedUnit(a)
		results[i].Unit = a.Unit
		results[i].Error = err
	}
	return results, nil
}

// UnitAssignments returns all staged unit assignments in the model.
func (st *State) AllUnitAssignments() ([]UnitAssignment, error) {
	return st.unitAssignments(nil)
}

func (st *State) unitAssignments(query bson.D) ([]UnitAssignment, error) {
	col, close := st.getCollection(assignUnitC)
	defer close()

	var docs []assignUnitDoc
	if err := col.Find(query).All(&docs); err != nil {
		return nil, errors.Annotatef(err, "cannot get unit assignment docs")
	}
	results := make([]UnitAssignment, len(docs))
	for i, doc := range docs {
		results[i] = UnitAssignment{
			st.localID(doc.DocId),
			doc.Scope,
			doc.Directive,
		}
	}
	return results, nil
}

func removeStagedAssignmentOp(id string) txn.Op {
	return txn.Op{
		C:      assignUnitC,
		Id:     id,
		Remove: true,
	}
}

func (st *State) assignStagedUnit(a UnitAssignment) error {
	u, err := st.Unit(a.Unit)
	if err != nil {
		return errors.Trace(err)
	}
	svc, err := u.Service()
	if err != nil {
		return errors.Trace(err)
	}
	networks, err := svc.Networks()
	if err != nil {
		return errors.Trace(err)
	}
	if a.Scope == "" && a.Directive == "" {
		return errors.Trace(st.AssignUnit(u, AssignCleanEmpty))
	}

	placement := &instance.Placement{Scope: a.Scope, Directive: a.Directive}

	// units always have the same networks as their service.
	return errors.Trace(st.AssignUnitWithPlacement(u, placement, networks))
}

// AssignUnitWithPlacement chooses a machine using the given placement directive
// and then assigns the unit to it.
func (st *State) AssignUnitWithPlacement(unit *Unit, placement *instance.Placement, networks []string) error {
	// TODO(natefinch) this should be done as a single transaction, not two.
	// Mark https://launchpad.net/bugs/1506994 fixed when done.

	m, err := st.addMachineWithPlacement(unit, placement, networks)
	if err != nil {
		return errors.Trace(err)
	}
	return unit.AssignToMachine(m)
}

// placementData is a helper type that encodes some of the logic behind how an
// instance.Placement gets translated into a placement directive the providers
// understand.
type placementData struct {
	machineId     string
	directive     string
	containerType instance.ContainerType
}

type placementType int

const (
	containerPlacement placementType = iota
	directivePlacement
	machinePlacement
)

// placementType returns the type of placement that this data represents.
func (p placementData) placementType() placementType {
	if p.containerType != "" {
		return containerPlacement
	}
	if p.directive != "" {
		return directivePlacement
	}
	return machinePlacement
}

func (st *State) parsePlacement(placement *instance.Placement) (*placementData, error) {
	// Extract container type and parent from container placement directives.
	if container, err := instance.ParseContainerType(placement.Scope); err == nil {
		return &placementData{
			containerType: container,
			machineId:     placement.Directive,
		}, nil
	}
	switch placement.Scope {
	case st.ModelUUID():
		return &placementData{directive: placement.Directive}, nil
	case instance.MachineScope:
		return &placementData{machineId: placement.Directive}, nil
	default:
		return nil, errors.Errorf("placement scope: invalid model UUID %q", placement.Scope)
	}
}

// addMachineWithPlacement finds a machine that matches the given placement directive for the given unit.
func (st *State) addMachineWithPlacement(unit *Unit, placement *instance.Placement, networks []string) (*Machine, error) {
	unitCons, err := unit.Constraints()
	if err != nil {
		return nil, err
	}

	data, err := st.parsePlacement(placement)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Create any new machine marked as dirty so that
	// nothing else will grab it before we assign the unit to it.
	// TODO(natefinch) fix this when we put assignment in the same
	// transaction as adding a machine.  See bug
	// https://launchpad.net/bugs/1506994

	switch data.placementType() {
	case containerPlacement:
		// If a container is to be used, create it.
		template := MachineTemplate{
			Series:            unit.Series(),
			Jobs:              []MachineJob{JobHostUnits},
			Dirty:             true,
			Constraints:       *unitCons,
			RequestedNetworks: networks,
		}
		return st.AddMachineInsideMachine(template, data.machineId, data.containerType)
	case directivePlacement:
		// If a placement directive is to be used, do that here.
		template := MachineTemplate{
			Series:            unit.Series(),
			Jobs:              []MachineJob{JobHostUnits},
			Dirty:             true,
			Constraints:       *unitCons,
			RequestedNetworks: networks,
			Placement:         data.directive,
		}
		return st.AddOneMachine(template)
	default:
		// Otherwise use an existing machine.
		return st.Machine(data.machineId)
	}
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
	var modelLocalProviderID string
	if args.ProviderId != "" {
		modelLocalProviderID = st.docID(string(args.ProviderId))
	}

	subDoc := subnetDoc{
		DocID:             subnetID,
		ModelUUID:         st.ModelUUID(),
		Life:              Alive,
		CIDR:              args.CIDR,
		VLANTag:           args.VLANTag,
		ProviderId:        modelLocalProviderID,
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
		assertModelAliveOp(st.ModelUUID()),
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
		if err := checkModeLife(st); err != nil {
			return nil, errors.Trace(err)
		}
		if _, err = st.Subnet(args.CIDR); err == nil {
			return nil, errors.AlreadyExistsf("subnet %q", args.CIDR)
		} else if err != nil {
			return nil, errors.Trace(err)
		}
	case nil:
		// If the ProviderId was not unique adding the subnet can fail without
		// an error. Refreshing catches this by returning NotFoundError.
		err = subnet.Refresh()
		if err != nil {
			if errors.IsNotFound(err) {
				return nil, errors.Errorf("ProviderId %q not unique", args.ProviderId)
			}
			return nil, errors.Trace(err)
		}
		return subnet, nil
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

// AllSubnets returns all known subnets in the model.
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
		assertModelAliveOp(st.ModelUUID()),
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
		if err := checkModeLife(st); err != nil {
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

// AllNetworks returns all known networks in the model.
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

// AllServices returns all deployed services in the model.
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
// where the model uuid is prefixed to the
// localID.
func (st *State) docID(localID string) string {
	return ensureModelUUID(st.ModelUUID(), localID)
}

// localID returns the local id value by stripping
// off the model uuid prefix if it is there.
func (st *State) localID(ID string) string {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.ModelUUID() {
		return ID
	}
	return localID
}

// strictLocalID returns the local id value by removing the
// model UUID prefix.
//
// If there is no prefix matching the State's model, an error is
// returned.
func (st *State) strictLocalID(ID string) (string, error) {
	modelUUID, localID, ok := splitDocID(ID)
	if !ok || modelUUID != st.ModelUUID() {
		return "", errors.Errorf("unexpected id: %#v", ID)
	}
	return localID, nil
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
			ModelUUID: st.ModelUUID(),
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

// AllRelations returns all relations in the model ordered by id.
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
// state of the model, this may lead to new instances being launched
// within the model.
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

type controllersDoc struct {
	Id               string `bson:"_id"`
	ModelUUID        string `bson:"model-uuid"`
	MachineIds       []string
	VotingMachineIds []string
}

// ControllerInfo holds information about currently
// configured controller machines.
type ControllerInfo struct {
	// ModelTag identifies the initial model. Only the initial
	// model is able to have machines that manage state. The initial
	// model is the model that is created when bootstrapping.
	ModelTag names.ModelTag

	// MachineIds holds the ids of all machines configured
	// to run a controller. It includes all the machine
	// ids in VotingMachineIds.
	MachineIds []string

	// VotingMachineIds holds the ids of all machines
	// configured to run a controller and to have a vote
	// in peer election.
	VotingMachineIds []string
}

// ControllerInfo returns information about
// the currently configured controller machines.
func (st *State) ControllerInfo() (*ControllerInfo, error) {
	session := st.session.Copy()
	defer session.Close()
	return readRawControllerInfo(st.session)
}

// readRawControllerInfo reads ControllerInfo direct from the supplied session,
// falling back to the bootstrap model document to extract the UUID when
// required.
func readRawControllerInfo(session *mgo.Session) (*ControllerInfo, error) {
	db := session.DB(jujuDB)
	controllers := db.C(controllersC)

	var doc controllersDoc
	err := controllers.Find(bson.D{{"_id", modelGlobalKey}}).One(&doc)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get controllers document")
	}

	if doc.ModelUUID == "" {
		logger.Warningf("controllers info has no model UUID so retrieving it from model")

		// This only happens when migrating from 1.20 to 1.21 before
		// upgrade steps have been run. Without this hack modelTag
		// on State ends up empty, breaking basic functionality needed
		// to run upgrade steps (a chicken-and-egg scenario).
		environments := db.C(modelsC)

		var envDoc modelDoc
		query := environments.Find(nil)
		count, err := query.Count()
		if err != nil {
			return nil, errors.Annotate(err, "cannot get model document count")
		}
		if count != 1 {
			return nil, errors.New("expected just one model to get UUID from")
		}
		if err := query.One(&envDoc); err != nil {
			return nil, errors.Annotate(err, "cannot load model document")
		}
		doc.ModelUUID = envDoc.UUID
	}

	return &ControllerInfo{
		ModelTag:         names.NewModelTag(doc.ModelUUID),
		MachineIds:       doc.MachineIds,
		VotingMachineIds: doc.VotingMachineIds,
	}, nil
}

const stateServingInfoKey = "stateServingInfo"

// StateServingInfo returns information for running a controller machine
func (st *State) StateServingInfo() (StateServingInfo, error) {
	controllers, closer := st.getCollection(controllersC)
	defer closer()

	var info StateServingInfo
	err := controllers.Find(bson.D{{"_id", stateServingInfoKey}}).One(&info)
	if err != nil {
		return info, errors.Trace(err)
	}
	if info.StatePort == 0 {
		return StateServingInfo{}, errors.NotFoundf("state serving info")
	}
	return info, nil
}

// SetStateServingInfo stores information needed for running a controller
func (st *State) SetStateServingInfo(info StateServingInfo) error {
	if info.StatePort == 0 || info.APIPort == 0 ||
		info.Cert == "" || info.PrivateKey == "" {
		return errors.Errorf("incomplete state serving info set in state")
	}
	if info.CAPrivateKey == "" {
		// No CA certificate key means we can't generate new controller
		// certificates when needed to add to the certificate SANs.
		// Older Juju deployments discard the key because no one realised
		// the certificate was flawed, so at best we can log a warning
		// until an upgrade process is written.
		logger.Warningf("state serving info has no CA certificate key")
	}
	ops := []txn.Op{{
		C:      controllersC,
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
		C:      controllersC,
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
	'e': names.ModelTagKind + "-",
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
