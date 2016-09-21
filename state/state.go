// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state enables reading, observing, and changing
// the state stored in MongoDB of a whole model
// managed by juju.
package state

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/utils/clock"
	"github.com/juju/utils/os"
	"github.com/juju/utils/series"
	"github.com/juju/utils/set"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6-unstable"
	csparams "gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/audit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state/cloudimagemetadata"
	stateaudit "github.com/juju/juju/state/internal/audit"
	statelease "github.com/juju/juju/state/lease"
	"github.com/juju/juju/state/workers"
	"github.com/juju/juju/status"
	jujuversion "github.com/juju/juju/version"
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

	// applicationLeadershipNamespace is the name of the lease.Client namespace
	// used by the leadership manager.
	applicationLeadershipNamespace = "application-leadership"

	// singularControllerNamespace is the name of the lease.Client namespace
	// used by the singular manager
	singularControllerNamespace = "singular-controller"
)

type providerIdDoc struct {
	ID string `bson:"_id"` // format: "<model-uuid>:<global-key>:<provider-id>"
}

// State represents the state of an model
// managed by juju.
type State struct {
	clock              clock.Clock
	modelTag           names.ModelTag
	controllerModelTag names.ModelTag
	controllerTag      names.ControllerTag
	mongoInfo          *mongo.MongoInfo
	session            *mgo.Session
	database           Database
	policy             Policy
	newPolicy          NewPolicyFunc

	// cloudName is the name of the cloud on which the model
	// represented by this state runs.
	cloudName string

	// leaseClientId is used by the lease infrastructure to
	// differentiate between machines whose clocks may be
	// relatively-skewed.
	leaseClientId string

	// workers is responsible for keeping the various sub-workers
	// available by starting new ones as they fail. It doesn't do
	// that yet, but having a type that collects them together is the
	// first step.
	//
	// note that the allManager stuff below probably ought to be
	// folded in as well, but that feels like its own task.
	workers workers.Workers

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
//
// NOTE(fwereade): the api/params type exists *purely* for representing
// this data over the wire, and has a legitimate reason to exist. This
// type does not: it's non-implementation-specific and shoudl be defined
// under core/ somewhere, so it can be used both here and in the agent
// without dragging unnecessary/irrelevant packages into scope.
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
	return st.modelTag == st.controllerModelTag
}

// ControllerUUID returns the UUID for the controller
// of this state instance.
func (st *State) ControllerUUID() string {
	return st.controllerTag.Id()
}
func (st *State) ControllerTag() names.ControllerTag {
	return st.controllerTag
}

func ControllerAccess(st *State, tag names.Tag) (permission.UserAccess, error) {
	return st.UserAccess(tag.(names.UserTag), st.controllerTag)
}

// RemoveAllModelDocs removes all documents from multi-model
// collections. The model should be put into a dying state before call
// this method. Otherwise, there is a race condition in which collections
// could be added to during or after the running of this method.
func (st *State) RemoveAllModelDocs() error {
	err := st.removeAllModelDocs(bson.D{{"life", Dead}})
	if errors.Cause(err) == txn.ErrAborted {
		return errors.New("can't remove model: model not dead")
	}
	return errors.Trace(err)
}

// RemoveImportingModelDocs removes all documents from multi-model collections
// for the current model. This method asserts that the model's migration mode
// is "importing".
func (st *State) RemoveImportingModelDocs() error {
	err := st.removeAllModelDocs(bson.D{{"migration-mode", MigrationModeImporting}})
	if errors.Cause(err) == txn.ErrAborted {
		return errors.New("can't remove model: model not being imported for migration")
	}
	return errors.Trace(err)
}

// RemoveExportingModelDocs removes all documents from multi-model collections
// for the current model. This method asserts that the model's migration mode
// is "exporting".
func (st *State) RemoveExportingModelDocs() error {
	err := st.removeAllModelDocs(bson.D{{"migration-mode", MigrationModeExporting}})
	if errors.Cause(err) == txn.ErrAborted {
		return errors.New("can't remove model: model not being exported for migration")
	}
	return errors.Trace(err)
}

func (st *State) removeAllModelDocs(modelAssertion bson.D) error {
	modelUUID := st.ModelUUID()

	// Remove each collection in its own transaction.
	for name, info := range st.database.Schema() {
		if info.global || info.rawAccess {
			continue
		}

		ops, err := st.removeAllInCollectionOps(name)
		if err != nil {
			return errors.Trace(err)
		}
		// Make sure we gate everything on the model assertion.
		ops = append([]txn.Op{{
			C:      modelsC,
			Id:     modelUUID,
			Assert: modelAssertion,
		}}, ops...)
		err = st.runTransaction(ops)
		if err != nil {
			return errors.Trace(err)
		}
	}

	// Remove from the raw (non-transactional) collections.
	for name, info := range st.database.Schema() {
		if !info.global && info.rawAccess {
			if err := st.removeAllInCollectionRaw(name); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Remove all user permissions for the model.
	permPattern := bson.M{
		"_id": bson.M{"$regex": "^" + permissionID(modelKey(modelUUID), "")},
	}
	ops, err := st.removeInCollectionOps(permissionsC, permPattern)
	if err != nil {
		return errors.Trace(err)
	}
	err = st.runTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}

	// Now remove remove the model.
	env, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	id := userModelNameIndex(env.Owner().Canonical(), env.Name())
	ops = []txn.Op{{
		// Cleanup the owner:envName unique key.
		C:      usermodelnameC,
		Id:     id,
		Remove: true,
	}, {
		C:      modelEntityRefsC,
		Id:     modelUUID,
		Remove: true,
	}, {
		C:      modelsC,
		Id:     modelUUID,
		Assert: modelAssertion,
		Remove: true,
	}}
	if !st.IsController() {
		ops = append(ops, decHostedModelCountOp())
	}
	return st.runTransaction(ops)
}

// removeAllInCollectionRaw removes all the documents from the given
// named collection.
func (st *State) removeAllInCollectionRaw(name string) error {
	coll, closer := st.getCollection(name)
	defer closer()
	_, err := coll.Writeable().RemoveAll(nil)
	return errors.Trace(err)
}

// removeAllInCollectionOps appends to ops operations to
// remove all the documents in the given named collection.
func (st *State) removeAllInCollectionOps(name string) ([]txn.Op, error) {
	return st.removeInCollectionOps(name, nil)
}

// removeInCollectionOps generates operations to remove all documents
// from the named collection matching a specific selector.
func (st *State) removeInCollectionOps(name string, sel interface{}) ([]txn.Op, error) {
	coll, closer := st.getCollection(name)
	defer closer()

	var ids []bson.M
	err := coll.Find(sel).Select(bson.D{{"_id", 1}}).All(&ids)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var ops []txn.Op
	for _, id := range ids {
		ops = append(ops, txn.Op{
			C:      name,
			Id:     id["_id"],
			Remove: true,
		})
	}
	return ops, nil
}

// ForModel returns a connection to mongo for the specified model. The
// connection uses the same credentials and policy as the existing connection.
func (st *State) ForModel(modelTag names.ModelTag) (*State, error) {
	session := st.session.Copy()
	newSt, err := newState(
		modelTag, st.controllerModelTag, session, st.mongoInfo, st.newPolicy, st.clock,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := newSt.start(st.controllerTag); err != nil {
		return nil, errors.Trace(err)
	}
	return newSt, nil
}

// start makes a *State functional post-creation, by:
//   * setting controllerTag, cloudName and leaseClientId
//   * starting lease managers and watcher backends
//   * creating cloud metadata storage
//
// start will close the *State if it fails.
func (st *State) start(controllerTag names.ControllerTag) (err error) {
	defer func() {
		if err == nil {
			return
		}
		if err2 := st.Close(); err2 != nil {
			logger.Errorf("closing State for %s: %v", st.modelTag, err2)
		}
	}()

	st.controllerTag = controllerTag

	if identity := st.mongoInfo.Tag; identity != nil {
		// TODO(fwereade): it feels a bit wrong to take this from MongoInfo -- I
		// think it's just coincidental that the mongodb user happens to map to
		// the machine that's executing the code -- but there doesn't seem to be
		// an accessible alternative.
		st.leaseClientId = identity.String()
	} else {
		// If we're running state anonymously, we can still use the lease
		// manager; but we need to make sure we use a unique client ID, and
		// will thus not be very performant.
		logger.Infof("running state anonymously; using unique client id")
		uuid, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		st.leaseClientId = fmt.Sprintf("anon-%s", uuid.String())
	}
	// now we've set up leaseClientId, we can use workersFactory

	logger.Infof("starting standard state workers")
	factory := workersFactory{
		st:    st,
		clock: st.clock,
	}
	workers, err := workers.NewRestartWorkers(workers.RestartConfig{
		Factory: factory,
		Logger:  loggo.GetLogger(logger.Name() + ".workers"),
		Clock:   st.clock,
		Delay:   time.Second,
	})
	if err != nil {
		return errors.Annotatef(err, "cannot create standard state workers")
	}
	st.workers = workers

	logger.Infof("creating cloud image metadata storage")
	st.CloudImageMetadataStorage = cloudimagemetadata.NewStorage(
		cloudimagemetadataC,
		&environMongo{st},
	)

	logger.Infof("started state for %s successfully", st.modelTag)
	return nil
}

func (st *State) getLeadershipLeaseClient() (lease.Client, error) {
	client, err := statelease.NewClient(statelease.ClientConfig{
		Id:         st.leaseClientId,
		Namespace:  applicationLeadershipNamespace,
		Collection: leasesC,
		Mongo:      &environMongo{st},
		Clock:      st.clock,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create leadership lease client")
	}
	return client, nil
}

func (st *State) getSingularLeaseClient() (lease.Client, error) {
	client, err := statelease.NewClient(statelease.ClientConfig{
		Id:         st.leaseClientId,
		Namespace:  singularControllerNamespace,
		Collection: leasesC,
		Mongo:      &environMongo{st},
		Clock:      st.clock,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create singular lease client")
	}
	return client, nil
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

// getPresenceCollection returns the raw mongodb presence collection,
// which is needed to interact with the state/presence package.
func (st *State) getPresenceCollection() *mgo.Collection {
	return st.session.DB(presenceDB).C(presenceC)
}

// getTxnLogCollection returns the raw mongodb txns collection, which is
// needed to interact with the state/watcher package.
func (st *State) getTxnLogCollection() *mgo.Collection {
	return st.session.DB(jujuDB).C(txnLogC)
}

// newDB returns a database connection using a new session, along with
// a closer function for the session. This is useful where you need to work
// with various collections in a single session, so don't want to call
// getCollection multiple times.
func (st *State) newDB() (Database, func()) {
	return st.database.Copy()
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

// IsUpgradeInProgressError returns true if the error is caused by an
// in-progress upgrade.
func IsUpgradeInProgressError(err error) bool {
	return errors.Cause(err) == errUpgradeInProgress
}

// SetModelAgentVersion changes the agent version for the model to the
// given version, only if the model is in a stable state (all agents are
// running the current version). If this is a hosted model, newVersion
// cannot be higher than the controller version.
func (st *State) SetModelAgentVersion(newVersion version.Number) (err error) {
	if newVersion.Compare(jujuversion.Current) > 0 && !st.IsController() {
		return errors.Errorf("a hosted model cannot have a higher version than the server model: %s > %s",
			newVersion.String(),
			jujuversion.Current,
		)
	}

	buildTxn := func(attempt int) ([]txn.Op, error) {
		settings, err := readSettings(st, settingsC, modelGlobalKey)
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

// ModelConstraints returns the current model constraints.
func (st *State) ModelConstraints() (constraints.Value, error) {
	cons, err := readConstraints(st, modelGlobalKey)
	return cons, errors.Trace(err)
}

// SetModelConstraints replaces the current model constraints.
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

func (st *State) allMachines(machinesCollection mongo.Collection) ([]*Machine, error) {
	mdocs := machineDocSlice{}
	err := machinesCollection.Find(nil).All(&mdocs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all machines")
	}
	sort.Sort(mdocs)
	machines := make([]*Machine, len(mdocs))
	for i, doc := range mdocs {
		machines[i] = newMachine(st, &doc)
	}
	return machines, nil
}

// AllMachines returns all machines in the model
// ordered by id.
func (st *State) AllMachines() ([]*Machine, error) {
	machinesCollection, closer := st.getCollection(machinesC)
	defer closer()
	return st.allMachines(machinesCollection)
}

// AllMachinesFor returns all machines for the model represented
// by the given modeluuid
func (st *State) AllMachinesFor(modelUUID string) ([]*Machine, error) {
	machinesCollection, closer := st.getCollectionFor(modelUUID, machinesC)
	defer closer()
	return st.allMachines(machinesCollection)
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
	machinesCollection, closer := st.getCollection(machinesC)
	defer closer()

	var err error
	mdoc := &machineDoc{}
	err = machinesCollection.FindId(id).One(mdoc)

	switch err {
	case nil:
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
	case names.ApplicationTag:
		return st.Application(id)
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
	case names.ApplicationTag:
		coll = applicationsC
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

// addPeerRelationsOps returns the operations necessary to add the
// specified service peer relations to the state.
func (st *State) addPeerRelationsOps(applicationname string, peers map[string]charm.Relation) ([]txn.Op, error) {
	var ops []txn.Op
	for _, rel := range peers {
		relId, err := st.sequence("relation")
		if err != nil {
			return nil, errors.Trace(err)
		}
		eps := []Endpoint{{
			ApplicationName: applicationname,
			Relation:        rel,
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

type AddApplicationArgs struct {
	Name             string
	Series           string
	Charm            *Charm
	Channel          csparams.Channel
	Storage          map[string]StorageConstraints
	EndpointBindings map[string]string
	Settings         charm.Settings
	NumUnits         int
	Placement        []*instance.Placement
	Constraints      constraints.Value
	Resources        map[string]string
}

// AddApplication creates a new application, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddApplication(args AddApplicationArgs) (_ *Application, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add application %q", args.Name)
	// Sanity checks.
	if !names.IsValidApplication(args.Name) {
		return nil, errors.Errorf("invalid name")
	}
	if args.Charm == nil {
		return nil, errors.Errorf("charm is nil")
	}

	if err := validateCharmVersion(args.Charm); err != nil {
		return nil, errors.Trace(err)
	}

	if exists, err := isNotDead(st, applicationsC, args.Name); err != nil {
		return nil, errors.Trace(err)
	} else if exists {
		return nil, errors.Errorf("application already exists")
	}
	if err := checkModelActive(st); err != nil {
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
		if len(supportedSeries) > 0 {
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
					"series %q (OS %q) not supported by charm, supported series are %q",
					args.Series, seriesOS, strings.Join(supportedSeries, ", "),
				), "")
			}
		}
	}

	// Ignore constraints that result from this call as
	// these would be accumulation of model and application constraints
	// but we only want application constraints to be persisted here.
	_, err = st.resolveConstraints(args.Constraints)
	if err != nil {
		return nil, errors.Trace(err)
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

	applicationID := st.docID(args.Name)

	// Create the service addition operations.
	peers := args.Charm.Meta().Peers

	// The doc defaults to CharmModifiedVersion = 0, which is correct, since it
	// has, by definition, at its initial state.
	svcDoc := &applicationDoc{
		DocID:         applicationID,
		Name:          args.Name,
		ModelUUID:     st.ModelUUID(),
		Series:        args.Series,
		Subordinate:   args.Charm.Meta().Subordinate,
		CharmURL:      args.Charm.URL(),
		Channel:       string(args.Channel),
		RelationCount: len(peers),
		Life:          Alive,
	}

	svc := newApplication(st, svcDoc)

	endpointBindingsOp, err := createEndpointBindingsOp(
		st, svc.globalKey(),
		args.EndpointBindings, args.Charm.Meta(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	statusDoc := statusDoc{
		ModelUUID:  st.ModelUUID(),
		Status:     status.Waiting,
		StatusInfo: status.MessageWaitForMachine,
		Updated:    st.clock.Now().UnixNano(),
		// This exists to preserve questionable unit-aggregation behaviour
		// while we work out how to switch to an implementation that makes
		// sense. It is also set in AddMissingServiceStatuses.
		NeverSet: true,
	}

	// The addServiceOps does not include the environment alive assertion,
	// so we add it here.
	ops := []txn.Op{
		assertModelActiveOp(st.ModelUUID()),
		endpointBindingsOp,
	}
	addOps, err := addApplicationOps(st, addApplicationOpsArgs{
		applicationDoc: svcDoc,
		statusDoc:      statusDoc,
		constraints:    args.Constraints,
		storage:        args.Storage,
		settings:       map[string]interface{}(args.Settings),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	ops = append(ops, addOps...)

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
		unitName, unitOps, err := svc.addServiceUnitOps(applicationAddUnitOpsArgs{cons: args.Constraints, storageCons: args.Storage})
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
		if err := checkModelActive(st); err != nil {
			return nil, errors.Trace(err)
		}
		// TODO(fwereade): 2016-09-09 lp:1621754
		// This is not always correct -- there are a million
		// operations collected in this func, not *all* of them
		// imply that this is the problem. (e.g. the charm being
		// destroyed just as we add application will fail, but
		// not because "application already exists")
		return nil, errors.Errorf("application already exists")
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
var AddServicePostFuncs = map[string]func(*State, AddApplicationArgs) error{}

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
	if a.Scope == "" && a.Directive == "" {
		return errors.Trace(st.AssignUnit(u, AssignCleanEmpty))
	}

	placement := &instance.Placement{Scope: a.Scope, Directive: a.Directive}

	return errors.Trace(st.AssignUnitWithPlacement(u, placement))
}

// AssignUnitWithPlacement chooses a machine using the given placement directive
// and then assigns the unit to it.
func (st *State) AssignUnitWithPlacement(unit *Unit, placement *instance.Placement) error {
	// TODO(natefinch) this should be done as a single transaction, not two.
	// Mark https://launchpad.net/bugs/1506994 fixed when done.

	m, err := st.addMachineWithPlacement(unit, placement)
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
func (st *State) addMachineWithPlacement(unit *Unit, placement *instance.Placement) (*Machine, error) {
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
			Series:      unit.Series(),
			Jobs:        []MachineJob{JobHostUnits},
			Dirty:       true,
			Constraints: *unitCons,
		}
		if data.machineId != "" {
			return st.AddMachineInsideMachine(template, data.machineId, data.containerType)
		}
		return st.AddMachineInsideNewMachine(template, template, data.containerType)
	case directivePlacement:
		// If a placement directive is to be used, do that here.
		template := MachineTemplate{
			Series:      unit.Series(),
			Jobs:        []MachineJob{JobHostUnits},
			Dirty:       true,
			Constraints: *unitCons,
			Placement:   data.directive,
		}
		return st.AddOneMachine(template)
	default:
		// Otherwise use an existing machine.
		return st.Machine(data.machineId)
	}
}

// Service returns a service state by name.
func (st *State) Application(name string) (_ *Application, err error) {
	applications, closer := st.getCollection(applicationsC)
	defer closer()

	if !names.IsValidApplication(name) {
		return nil, errors.Errorf("%q is not a valid application name", name)
	}
	sdoc := &applicationDoc{}
	err = applications.FindId(name).One(sdoc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("application %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get application %q", name)
	}
	return newApplication(st, sdoc), nil
}

// AllApplications returns all deployed services in the model.
func (st *State) AllApplications() (applications []*Application, err error) {
	applicationsCollection, closer := st.getCollection(applicationsC)
	defer closer()

	sdocs := []applicationDoc{}
	err = applicationsCollection.Find(bson.D{}).All(&sdocs)
	if err != nil {
		return nil, errors.Errorf("cannot get all applications")
	}
	for _, v := range sdocs {
		applications = append(applications, newApplication(st, &v))
	}
	return applications, nil
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
		svc, err := st.Application(ep.ApplicationName)
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
	svc, err := st.Application(svcName)
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
			svc, err := st.Application(ep.ApplicationName)
			if errors.IsNotFound(err) {
				return nil, errors.Errorf("application %q does not exist", ep.ApplicationName)
			} else if err != nil {
				return nil, errors.Trace(err)
			} else if svc.doc.Life != Alive {
				return nil, errors.Errorf("application %q is not alive", ep.ApplicationName)
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
				return nil, errors.Errorf("%q does not implement %q", ep.ApplicationName, ep)
			}
			ops = append(ops, txn.Op{
				C:      applicationsC,
				Id:     st.docID(ep.ApplicationName),
				Assert: bson.D{{"life", Alive}, {"charmurl", ch.URL()}},
				Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
			})
		}
		if matchSeries && len(series) != 1 {
			return nil, errors.Errorf("principal and subordinate applications' series must match")
		}
		if eps[0].Scope == charm.ScopeContainer && subordinateCount < 1 {
			return nil, errors.Errorf("container scoped relation requires at least one subordinate application")
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
	st.workers.TxnLogWatcher().StartSync()
	st.workers.PresenceWatcher().Sync()
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
	CloudName        string `bson:"cloud"`
	ModelUUID        string `bson:"model-uuid"`
	MachineIds       []string
	VotingMachineIds []string
	MongoSpaceName   string `bson:"mongo-space-name"`
	MongoSpaceState  string `bson:"mongo-space-state"`
}

// ControllerInfo holds information about currently
// configured controller machines.
type ControllerInfo struct {
	// CloudName is the name of the cloud to which this controller is deployed.
	CloudName string

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

	// MongoSpaceName is the space that contains all Mongo servers.
	MongoSpaceName string

	// MongoSpaceState records the state of the mongo space selection state machine. Valid states are:
	// * We haven't looked for a Mongo space yet (MongoSpaceUnknown)
	// * We have looked for a Mongo space, but we didn't find one (MongoSpaceInvalid)
	// * We have looked for and found a Mongo space (MongoSpaceValid)
	// * We didn't try to find a Mongo space because the provider doesn't support spaces (MongoSpaceUnsupported)
	MongoSpaceState MongoSpaceStates
}

type MongoSpaceStates string

const (
	MongoSpaceUnknown     MongoSpaceStates = ""
	MongoSpaceValid       MongoSpaceStates = "valid"
	MongoSpaceInvalid     MongoSpaceStates = "invalid"
	MongoSpaceUnsupported MongoSpaceStates = "unsupported"
)

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
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("controllers document")
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get controllers document")
	}
	return &ControllerInfo{
		CloudName:        doc.CloudName,
		ModelTag:         names.NewModelTag(doc.ModelUUID),
		MachineIds:       doc.MachineIds,
		VotingMachineIds: doc.VotingMachineIds,
		MongoSpaceName:   doc.MongoSpaceName,
		MongoSpaceState:  MongoSpaceStates(doc.MongoSpaceState),
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

// SetOrGetMongoSpaceName attempts to set the Mongo space or, if that fails, look
// up the current Mongo space. Either way, it always returns what is in the
// database by the end of the call.
func (st *State) SetOrGetMongoSpaceName(mongoSpaceName network.SpaceName) (network.SpaceName, error) {
	err := st.setMongoSpaceName(mongoSpaceName)
	if err == txn.ErrAborted {
		// Failed to set the new space name. Return what is already stored in state.
		controllerInfo, err := st.ControllerInfo()
		if err != nil {
			return network.SpaceName(""), errors.Trace(err)
		}
		return network.SpaceName(controllerInfo.MongoSpaceName), nil
	} else if err != nil {
		return network.SpaceName(""), errors.Trace(err)
	}
	return mongoSpaceName, nil
}

// SetMongoSpaceState attempts to set the Mongo space state or, if that fails, look
// up the current Mongo state. Either way, it always returns what is in the
// database by the end of the call.
func (st *State) SetMongoSpaceState(mongoSpaceState MongoSpaceStates) error {

	if mongoSpaceState != MongoSpaceUnknown &&
		mongoSpaceState != MongoSpaceValid &&
		mongoSpaceState != MongoSpaceInvalid &&
		mongoSpaceState != MongoSpaceUnsupported {
		return errors.NotValidf("mongoSpaceState: %s", mongoSpaceState)
	}

	err := st.setMongoSpaceState(mongoSpaceState)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (st *State) setMongoSpaceName(mongoSpaceName network.SpaceName) error {
	ops := []txn.Op{{
		C:      controllersC,
		Id:     modelGlobalKey,
		Assert: bson.D{{"mongo-space-state", string(MongoSpaceUnknown)}},
		Update: bson.D{{
			"$set",
			bson.D{
				{"mongo-space-name", string(mongoSpaceName)},
				{"mongo-space-state", MongoSpaceValid},
			},
		}},
	}}

	return st.runTransaction(ops)
}

func (st *State) setMongoSpaceState(mongoSpaceState MongoSpaceStates) error {
	ops := []txn.Op{{
		C:      controllersC,
		Id:     modelGlobalKey,
		Update: bson.D{{"$set", bson.D{{"mongo-space-state", mongoSpaceState}}}},
	}}

	return st.runTransaction(ops)
}

func (st *State) networkEntityGlobalKeyOp(globalKey string, providerId network.Id) txn.Op {
	key := st.networkEntityGlobalKey(globalKey, providerId)
	return txn.Op{
		C:      providerIDsC,
		Id:     key,
		Assert: txn.DocMissing,
		Insert: providerIdDoc{ID: key},
	}
}

func (st *State) networkEntityGlobalKeyRemoveOp(globalKey string, providerId network.Id) txn.Op {
	key := st.networkEntityGlobalKey(globalKey, providerId)
	return txn.Op{
		C:      providerIDsC,
		Id:     key,
		Remove: true,
	}
}

func (st *State) networkEntityGlobalKey(globalKey string, providerId network.Id) string {
	return st.docID(globalKey + ":" + string(providerId))
}

// PutAuditEntryFn returns a function which will persist
// audit.AuditEntry instances to the database.
func (st *State) PutAuditEntryFn() func(audit.AuditEntry) error {
	insert := func(collectionName string, docs ...interface{}) error {
		collection, closeCollection := st.getCollection(collectionName)
		defer closeCollection()

		writeableCollection := collection.Writeable()

		return errors.Trace(writeableCollection.Insert(docs...))
	}
	return stateaudit.PutAuditEntryFn(auditingC, insert)
}

var tagPrefix = map[byte]string{
	'm': names.MachineTagKind + "-",
	'a': names.ApplicationTagKind + "-",
	'u': names.UnitTagKind + "-",
	'e': names.ModelTagKind + "-",
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

// SetClockForTesting is an exported function to allow other packages
// to set the internal clock for the State instance. It is named such
// that it should be obvious if it is ever called from a non-test packgae.
func (st *State) SetClockForTesting(clock clock.Clock) error {
	st.clock = clock
	// Need to restart the lease workers so they get the new clock.
	st.workers.Kill()
	err := st.workers.Wait()
	if err != nil {
		return errors.Trace(err)
	}
	err = st.start(st.controllerTag)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
