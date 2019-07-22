// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package state enables reading, observing, and changing
// the state stored in MongoDB of a whole model
// managed by juju.
package state

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os"
	"github.com/juju/os/series"
	"github.com/juju/pubsub"
	jujutxn "github.com/juju/txn"
	"github.com/juju/utils"
	"github.com/juju/version"
	"gopkg.in/juju/charm.v6"
	csparams "gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/constraints"
	coreglobalclock "github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/state/globalclock"
	statelease "github.com/juju/juju/state/lease"
	"github.com/juju/juju/state/presence"
	raftleasestore "github.com/juju/juju/state/raftlease"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/storage"
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

	// applicationLeadershipNamespace is the name of the lease.Store namespace
	// used by the leadership manager.
	applicationLeadershipNamespace = "application-leadership"

	// singularControllerNamespace is the name of the lease.Store namespace
	// used by the singular manager
	singularControllerNamespace = "singular-controller"
)

type providerIdDoc struct {
	ID string `bson:"_id"` // format: "<model-uuid>:<global-key>:<provider-id>"
}

// State represents the state of an model
// managed by juju.
type State struct {
	stateClock             clock.Clock
	modelTag               names.ModelTag
	controllerModelTag     names.ModelTag
	controllerTag          names.ControllerTag
	session                *mgo.Session
	database               Database
	policy                 Policy
	newPolicy              NewPolicyFunc
	runTransactionObserver RunTransactionObserverFunc

	// leaseStoreId is used by the lease infrastructure to
	// differentiate between machines whose clocks may be
	// relatively-skewed.
	leaseStoreId string

	// workers is responsible for keeping the various sub-workers
	// available by starting new ones as they fail. It doesn't do
	// that yet, but having a type that collects them together is the
	// first step.
	workers *workers

	// TODO(anastasiamac 2015-07-16) As state gets broken up, remove this.
	CloudImageMetadataStorage cloudimagemetadata.Storage
}

// StateServingInfo holds information needed by a controller.
// This type is a copy of the type of the same name from the api/params package.
// It is replicated here to avoid the state package depending on api/params.
//
// NOTE(fwereade): the api/params type exists *purely* for representing
// this data over the wire, and has a legitimate reason to exist. This
// type does not: it's non-implementation-specific and should be defined
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

func (st *State) newStateNoWorkers(modelUUID string) (*State, error) {
	session := st.session.Copy()
	newSt, err := newState(
		names.NewModelTag(modelUUID),
		st.controllerModelTag,
		session,
		st.newPolicy,
		st.stateClock,
		st.runTransactionObserver,
	)
	// We explicitly don't start the workers.
	if err != nil {
		session.Close()
		return nil, errors.Trace(err)
	}
	return newSt, nil
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

// ControllerTag returns the tag form of the the return value of
// ControllerUUID.
func (st *State) ControllerTag() names.ControllerTag {
	return st.controllerTag
}

// ControllerTimestamp returns the current timestamp of the backend
// controller.
func (st *State) ControllerTimestamp() (*time.Time, error) {
	now := st.clock().Now()
	return &now, nil
}

// ControllerModelUUID returns the UUID of the model that was
// bootstrapped.  This is the only model that can have controller
// machines.  The owner of this model is also considered "special", in
// that they are the only user that is able to create other users
// (until we have more fine grained permissions), and they cannot be
// disabled.
func (st *State) ControllerModelUUID() string {
	return st.controllerModelTag.Id()
}

// ControllerModelTag returns the tag form the return value of
// ControllerModelUUID.
func (st *State) ControllerModelTag() names.ModelTag {
	return st.controllerModelTag
}

// ControllerOwner returns the owner of the controller model.
func (st *State) ControllerOwner() (names.UserTag, error) {
	models, closer := st.db().GetCollection(modelsC)
	defer closer()
	var doc map[string]string
	err := models.FindId(st.ControllerModelUUID()).Select(bson.M{"owner": 1}).One(&doc)
	if err != nil {
		return names.UserTag{}, errors.Annotate(err, "loading controller model")
	}
	owner := doc["owner"]
	if owner == "" {
		return names.UserTag{}, errors.New("model owner missing")
	}
	return names.NewUserTag(owner), nil
}

func ControllerAccess(st *State, tag names.Tag) (permission.UserAccess, error) {
	return st.UserAccess(tag.(names.UserTag), st.controllerTag)
}

// setDyingModelToDead sets current dying model to dead.
func (st *State) setDyingModelToDead() error {
	buildTxn := func(attempt int) ([]txn.Op, error) {
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if model.Life() != Dying {
			return nil, errors.Trace(ErrModelNotDying)
		}
		ops := []txn.Op{{
			C:      modelsC,
			Id:     st.ModelUUID(),
			Assert: isDyingDoc,
			Update: bson.M{"$set": bson.M{
				"life":          Dead,
				"time-of-death": st.nowToTheSecond(),
			}},
		}, {
			// Cleanup the owner:modelName unique key.
			C:      usermodelnameC,
			Id:     model.uniqueIndexID(),
			Remove: true,
		}}
		return ops, nil
	}
	if err := st.db().Run(buildTxn); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// RemoveDyingModel sets current model to dead then removes all documents from
// multi-model collections.
func (st *State) RemoveDyingModel() error {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	if model.Life() == Alive {
		return errors.Errorf("can't remove model: model still alive")
	}
	if model.Life() == Dying {
		// set model to dead if it's dying.
		if err = st.setDyingModelToDead(); err != nil {
			return errors.Trace(err)
		}
	}
	err = st.removeAllModelDocs(bson.D{{"life", Dead}})
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
		err = st.db().RunTransaction(ops)
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
	// Logs and presence are in separate databases so don't get caught by that
	// loop.
	removeModelLogs(st.MongoSession(), modelUUID)
	err := presence.RemovePresenceForModel(st.getPresenceCollection(), st.modelTag)
	if err != nil {
		return errors.Trace(err)
	}

	// Remove all user permissions for the model.
	permPattern := bson.M{
		"_id": bson.M{"$regex": "^" + permissionID(modelKey(modelUUID), "")},
	}
	ops, err := st.removeInCollectionOps(permissionsC, permPattern)
	if err != nil {
		return errors.Trace(err)
	}
	err = st.db().RunTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}

	// Now remove the model.
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	ops = []txn.Op{{
		// Cleanup the owner:envName unique key.
		C:      usermodelnameC,
		Id:     model.uniqueIndexID(),
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

	// Decrement the model count for the cloud to which this model belongs.
	decCloudRefOp, err := decCloudModelRefOp(st, model.Cloud())
	if err != nil {
		return errors.Trace(err)
	}
	ops = append(ops, decCloudRefOp)

	if !st.IsController() {
		ops = append(ops, decHostedModelCountOp())
	}
	return st.db().RunTransaction(ops)
}

// removeAllInCollectionRaw removes all the documents from the given
// named collection.
func (st *State) removeAllInCollectionRaw(name string) error {
	coll, closer := st.db().GetCollection(name)
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
	coll, closer := st.db().GetCollection(name)
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

// start makes a *State functional post-creation, by:
//   * setting controllerTag, cloudName and leaseStoreId
//   * starting lease managers and watcher backends
//   * creating cloud metadata storage
//
// start will close the *State if it fails.
func (st *State) start(controllerTag names.ControllerTag, hub *pubsub.SimpleHub) (err error) {
	defer func() {
		if err == nil {
			return
		}
		if err2 := st.Close(); err2 != nil {
			logger.Errorf("closing State for %s: %v", st.modelTag, err2)
		}
	}()

	st.controllerTag = controllerTag

	// Run the "connectionStatus" Mongo command to obtain the authenticated
	// user name, if any. This is used below for the lease store ID.
	// See: https://docs.mongodb.com/manual/reference/command/connectionStatus/
	//
	// TODO(axw) when we move the workers to a higher level state.Manager
	// type, we should pass in a tag that identifies the agent running the
	// worker. That can then be used to identify the lease manager.
	var connectionStatus struct {
		AuthInfo struct {
			AuthenticatedUsers []struct {
				User string `bson:"user"`
			} `bson:"authenticatedUsers"`
		} `bson:"authInfo"`
	}
	if err := st.session.DB(jujuDB).Run(bson.D{{"connectionStatus", 1}}, &connectionStatus); err != nil {
		return errors.Annotate(err, "obtaining connection status")
	}

	if len(connectionStatus.AuthInfo.AuthenticatedUsers) == 1 {
		st.leaseStoreId = connectionStatus.AuthInfo.AuthenticatedUsers[0].User
	} else {
		// If we're running state anonymously, we can still use the lease
		// manager; but we need to make sure we use a unique store ID, and
		// will thus not be very performant.
		logger.Infof("running state anonymously; using unique store id")
		uuid, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		st.leaseStoreId = fmt.Sprintf("anon-%s", uuid.String())
	}
	// now we've set up leaseStoreId, we can use workersFactory

	logger.Infof("starting standard state workers")
	workers, err := newWorkers(st, hub)
	if err != nil {
		return errors.Trace(err)
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

// ApplicationLeaders returns a map of the application name to the
// unit name that is the current leader.
func (st *State) ApplicationLeaders() (map[string]string, error) {
	config, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !config.Features().Contains(feature.LegacyLeases) {
		return raftleasestore.LeaseHolders(
			&environMongo{st},
			leaseHoldersC,
			lease.ApplicationLeadershipNamespace,
			st.ModelUUID(),
		)
	}
	store, err := st.getLeadershipLeaseStore()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leases := store.Leases()
	result := make(map[string]string, len(leases))
	for key, value := range leases {
		result[key.Lease] = value.Holder
	}
	return result, nil
}

func (st *State) getLeadershipLeaseStore() (lease.Store, error) {
	return st.getLeaseStore(lease.ApplicationLeadershipNamespace)
}

func (st *State) getSingularLeaseStore() (lease.Store, error) {
	return st.getLeaseStore(lease.SingularControllerNamespace)
}

func (st *State) getLeaseStore(namespace string) (lease.Store, error) {
	globalClock, err := st.globalClockReader()
	if err != nil {
		return nil, errors.Annotate(err, "getting global clock for lease store")
	}

	// NOTE(axw) due to the lease managers being embedded in State,
	// we cannot use "upgrade steps" to upgrade the leases prior
	// to their use. Thus we must run the lease upgrade here. When
	// we are able to extract the lease managers from State, we
	// should remove this.
	if err := migrateModelLeasesToGlobalTime(st); err != nil {
		return nil, errors.Trace(err)
	}

	store, err := statelease.NewStore(statelease.StoreConfig{
		Id:          st.leaseStoreId,
		Namespace:   namespace,
		ModelUUID:   st.modelUUID(),
		Collection:  leasesC,
		Mongo:       &environMongo{st},
		LocalClock:  st.stateClock,
		GlobalClock: globalClock,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create %q lease store", namespace)
	}
	return store, nil
}

// LeaseNotifyTarget returns a raftlease.NotifyTarget for storing
// lease changes in the database.
func (st *State) LeaseNotifyTarget(logDest io.Writer, errorLogger raftleasestore.Logger) raftlease.NotifyTarget {
	return raftleasestore.NewNotifyTarget(&environMongo{st}, leaseHoldersC, logDest, errorLogger)
}

// LeaseTrapdoorFunc returns a raftlease.TrapdoorFunc for checking
// lease state in a database.
func (st *State) LeaseTrapdoorFunc() raftlease.TrapdoorFunc {
	return raftleasestore.MakeTrapdoorFunc(&environMongo{st}, leaseHoldersC)
}

// ModelUUID returns the model UUID for the model
// controlled by this state instance.
func (st *State) ModelUUID() string {
	return st.modelTag.Id()
}

// userModelNameIndex returns a string to be used as a usermodelnameC unique index.
func userModelNameIndex(username, modelName string) string {
	return strings.ToLower(username) + ":" + modelName
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
		coll, closer := st.db().GetCollection(name)
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

// getPingBatcher returns the implementation of how we serialize Ping requests
// for agents to the database.
func (st *State) getPingBatcher() *presence.PingBatcher {
	return st.workers.pingBatcherWorker()
}

// getTxnLogCollection returns the raw mongodb txns collection, which is
// needed to interact with the state/watcher package.
func (st *State) getTxnLogCollection() *mgo.Collection {
	if st.Ping() != nil {
		st.session.Refresh()
	}
	return st.session.DB(jujuDB).C(txnLogC)
}

// newDB returns a database connection using a new session, along with
// a closer function for the session. This is useful where you need to work
// with various collections in a single session, so don't want to call
// getCollection multiple times.
func (st *State) newDB() (Database, func()) {
	return st.database.Copy()
}

// db returns the Database instance used by the State. It is part of
// the modelBackend interface.
func (st *State) db() Database {
	return st.database
}

// txnLogWatcher returns the TxnLogWatcher for the State. It is part
// of the modelBackend interface.
func (st *State) txnLogWatcher() watcher.BaseWatcher {
	return st.workers.txnLogWatcher()
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

// WatchParams defines config to control which
// entites are included when watching a model.
type WatchParams struct {
	// IncludeOffers controls whether application offers should be watched.
	IncludeOffers bool
}

func (st *State) Watch(params WatchParams) *Multiwatcher {
	return NewMultiwatcher(st.workers.allManager(params))
}

func (st *State) WatchAllModels(pool *StatePool) *Multiwatcher {
	return NewMultiwatcher(st.workers.allModelManager(pool))
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

func (st *State) checkCanUpgradeCAAS(currentVersion, newVersion string) error {
	// TODO(caas)
	return nil
}

func (st *State) checkCanUpgradeIAAS(currentVersion, newVersion string) error {
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
		collection, closer := st.db().GetCollection(name)
		defer closer()
		var doc struct {
			DocID string `bson:"_id"`
		}
		iter := collection.Find(sel).Select(bson.D{{"_id", 1}}).Iter()
		defer iter.Close()
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
func (st *State) SetModelAgentVersion(newVersion version.Number, ignoreAgentVersions bool) (err error) {
	if newVersion.Compare(jujuversion.Current) > 0 && !st.IsController() {
		return errors.Errorf("model cannot be upgraded to %s while the controller is %s: upgrade 'controller' model first",
			newVersion.String(),
			jujuversion.Current,
		)
	}

	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	isCAAS := model.Type() == ModelTypeCAAS
	buildTxn := func(attempt int) ([]txn.Op, error) {
		settings, err := readSettings(st.db(), settingsC, modelGlobalKey)
		if err != nil {
			return nil, errors.Annotatef(err, "model %q", st.modelTag.Id())
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

		if !ignoreAgentVersions {
			if isCAAS {
				if err := st.checkCanUpgradeCAAS(currentVersion, newVersion.String()); err != nil {
					return nil, errors.Trace(err)
				}
			} else {
				if err := st.checkCanUpgradeIAAS(currentVersion, newVersion.String()); err != nil {
					return nil, errors.Trace(err)
				}
			}
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
	if err = st.db().Run(buildTxn); err == jujutxn.ErrExcessiveContention {
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
	machinesCollection, closer := st.db().GetCollection(machinesC)
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
	machinesCollection, closer := st.db().GetCollection(machinesC)
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
// *User, *Application, *Model, or *Action, depending
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
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		// Return an invalid entity error if the requested model is not
		// the current one.
		if id != model.UUID() {
			if utils.IsValidUUIDString(id) {
				return nil, errors.NotFoundf("model %q", id)
			}
			// TODO(axw) 2013-12-04 #1257587
			// We should not accept model tags that do not match the
			// model's UUID. We accept anything for now, to cater
			// both for past usage, and for potentially supporting aliases.
			logger.Warningf("model-tag does not match current model UUID: %q != %q", id, model.UUID())
			conf, err := model.ModelConfig()
			if err != nil {
				logger.Warningf("ModelConfig failed: %v", err)
			} else if id != conf.Name() {
				logger.Warningf("model-tag does not match current model name: %q != %q", id, conf.Name())
			}
		}
		return model, nil
	case names.RelationTag:
		return st.KeyRelation(id)
	case names.ActionTag:
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return model.ActionByTag(tag)
	case names.CharmTag:
		if url, err := charm.ParseURL(id); err != nil {
			logger.Warningf("Parsing charm URL %q failed: %v", id, err)
			return nil, errors.NotFoundf("could not find charm %q in state", id)
		} else {
			return st.Charm(url)
		}
	case names.VolumeTag:
		sb, err := NewStorageBackend(st)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return sb.Volume(tag)
	case names.FilesystemTag:
		sb, err := NewStorageBackend(st)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return sb.Filesystem(tag)
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
			return "", nil, fmt.Errorf("%q is not a local user", tag.Id())
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
// specified application peer relations to the state.
func (st *State) addPeerRelationsOps(applicationname string, peers map[string]charm.Relation) ([]txn.Op, error) {
	now := st.clock().Now()
	var ops []txn.Op
	for _, rel := range peers {
		relId, err := sequence(st, "relation")
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
		relationStatusDoc := statusDoc{
			Status:    status.Joining,
			ModelUUID: st.ModelUUID(),
			Updated:   now.UnixNano(),
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     relDoc.DocID,
			Assert: txn.DocMissing,
			Insert: relDoc,
		}, createStatusOp(st, relationGlobalScope(relId), relationStatusDoc))
	}
	return ops, nil
}

var (
	errSameNameRemoteApplicationExists = errors.Errorf("remote application with same name already exists")
	errLocalApplicationExists          = errors.Errorf("application already exists")
)

// SaveCloudServiceArgs defines the arguments for SaveCloudService method.
type SaveCloudServiceArgs struct {
	// Id will be the application Name if it's a part of application,
	// and will be controller UUID for k8s a controller(controller does not have an application),
	// then is wrapped with applicationGlobalKey.
	Id         string
	ProviderId string
	Addresses  []network.Address

	Generation            int64
	DesiredScaleProtected bool
}

// SaveCloudService creates a cloud service.
func (st *State) SaveCloudService(args SaveCloudServiceArgs) (_ *CloudService, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add cloud service %q", args.ProviderId)

	doc := cloudServiceDoc{
		DocID:                 applicationGlobalKey(args.Id),
		ProviderId:            args.ProviderId,
		Addresses:             fromNetworkAddresses(args.Addresses, OriginProvider),
		Generation:            args.Generation,
		DesiredScaleProtected: args.DesiredScaleProtected,
	}
	buildTxn := func(int) ([]txn.Op, error) {
		return buildCloudServiceOps(st, doc)
	}

	if err := st.db().Run(buildTxn); err != nil {
		return nil, errors.Annotate(err, "failed to save cloud service")
	}
	// refresh then return updated CloudService.
	return newCloudService(st, &doc).CloudService()
}

// CloudService returns a cloud service state by Id.
func (st *State) CloudService(id string) (*CloudService, error) {
	svc := newCloudService(st, &cloudServiceDoc{DocID: st.docID(applicationGlobalKey(id))})
	return svc.CloudService()
}

type AddApplicationArgs struct {
	Name              string
	Series            string
	Charm             *Charm
	Channel           csparams.Channel
	Storage           map[string]StorageConstraints
	Devices           map[string]DeviceConstraints
	AttachStorage     []names.StorageTag
	EndpointBindings  map[string]string
	ApplicationConfig *application.Config
	CharmConfig       charm.Settings
	NumUnits          int
	Placement         []*instance.Placement
	Constraints       constraints.Value
	Resources         map[string]string
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

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateCharmSeries(model.Type(), args.Series, args.Charm); err != nil {
		return nil, errors.Trace(err)
	}

	// CAAS charms don't support volume/block storage yet.
	if model.Type() == ModelTypeCAAS {
		for name, charmStorage := range args.Charm.Meta().Storage {
			if storageKind(charmStorage.Type) != storage.StorageKindBlock {
				continue
			}
			var count uint64
			if arg, ok := args.Storage[name]; ok {
				count = arg.Count
			}
			if charmStorage.CountMin > 0 || count > 0 {
				return nil, errors.NotSupportedf("block storage on a Kubernetes model")
			}
		}
	}

	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return nil, errors.Errorf("AttachStorage is non-empty but NumUnits is %d, must be 1", args.NumUnits)
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

	// ensure storage
	if args.Storage == nil {
		args.Storage = make(map[string]StorageConstraints)
	}
	sb, err := NewStorageBackend(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := addDefaultStorageConstraints(sb, args.Storage, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateStorageConstraints(sb, args.Storage, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	storagePools := make(set.Strings)
	for _, storageParams := range args.Storage {
		storagePools.Add(storageParams.Pool)
	}

	// ensure Devices
	if args.Devices == nil {
		args.Devices = make(map[string]DeviceConstraints)
	}
	deviceb, err := NewDeviceBackend(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := validateDeviceConstraints(deviceb, args.Devices, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}

	// Perform model specific arg processing.
	scale := 0
	placement := ""
	switch model.Type() {
	case ModelTypeIAAS:
		if err := st.processIAASModelApplicationArgs(&args); err != nil {
			return nil, errors.Trace(err)
		}
	case ModelTypeCAAS:
		if err := st.processCAASModelApplicationArgs(&args); err != nil {
			return nil, errors.Trace(err)
		}
		scale = args.NumUnits
		if len(args.Placement) == 1 {
			placement = args.Placement[0].Directive
		}
	}

	applicationID := st.docID(args.Name)

	// Create the application addition operations.
	peers := args.Charm.Meta().Peers

	// The doc defaults to CharmModifiedVersion = 0, which is correct, since it
	// has, by definition, at its initial state.
	appDoc := &applicationDoc{
		DocID:         applicationID,
		Name:          args.Name,
		ModelUUID:     st.ModelUUID(),
		Series:        args.Series,
		Subordinate:   args.Charm.Meta().Subordinate,
		CharmURL:      args.Charm.URL(),
		Channel:       string(args.Channel),
		RelationCount: len(peers),
		Life:          Alive,

		// CAAS
		DesiredScale: scale,
		Placement:    placement,
	}

	app := newApplication(st, appDoc)

	endpointBindingsOp, err := createEndpointBindingsOp(
		st, app.globalKey(),
		args.EndpointBindings, args.Charm.Meta(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	statusDoc := statusDoc{
		ModelUUID:  st.ModelUUID(),
		Status:     status.Waiting,
		StatusInfo: status.MessageWaitForMachine,
		Updated:    st.clock().Now().UnixNano(),
		// This exists to preserve questionable unit-aggregation behaviour
		// while we work out how to switch to an implementation that makes
		// sense.
		NeverSet: true,
	}
	if model.Type() == ModelTypeCAAS {
		statusDoc.StatusInfo = status.MessageWaitForContainer
	}

	if err := args.ApplicationConfig.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	appConfigAttrs := args.ApplicationConfig.Attributes()

	// When creating the settings, we ignore nils.  In other circumstances, nil
	// means to delete the value (reset to default), so creating with nil should
	// mean to use the default, i.e. don't set the value.
	removeNils(args.CharmConfig)
	removeNils(appConfigAttrs)

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// If we've tried once already and failed, check that
		// model may have been destroyed.
		if attempt > 0 {
			if err := checkModelActive(st); err != nil {
				return nil, errors.Trace(err)
			}
			// Ensure a local application with the same name doesn't exist.
			if exists, err := isNotDead(st, applicationsC, args.Name); err != nil {
				return nil, errors.Trace(err)
			} else if exists {
				return nil, errLocalApplicationExists
			}
			// Ensure a remote application with the same name doesn't exist.
			if remoteExists, err := isNotDead(st, remoteApplicationsC, args.Name); err != nil {
				return nil, errors.Trace(err)
			} else if remoteExists {
				return nil, errSameNameRemoteApplicationExists
			}
		}
		// The addApplicationOps does not include the model alive assertion,
		// so we add it here.
		ops := []txn.Op{
			assertModelActiveOp(st.ModelUUID()),
			endpointBindingsOp,
		}
		addOps, err := addApplicationOps(st, app, addApplicationOpsArgs{
			applicationDoc:    appDoc,
			statusDoc:         statusDoc,
			constraints:       args.Constraints,
			storage:           args.Storage,
			devices:           args.Devices,
			applicationConfig: appConfigAttrs,
			charmConfig:       map[string]interface{}(args.CharmConfig),
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
			unitName, unitOps, err := app.addApplicationUnitOps(applicationAddUnitOpsArgs{
				cons:          args.Constraints,
				storageCons:   args.Storage,
				attachStorage: args.AttachStorage,
			})
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
		return ops, nil
	}
	// At the last moment before inserting the application, prime status history.
	probablyUpdateStatusHistory(st.db(), app.globalKey(), statusDoc)

	if err = st.db().Run(buildTxn); err == nil {
		// Refresh to pick the txn-revno.
		if err = app.Refresh(); err != nil {
			return nil, errors.Trace(err)
		}
		return app, nil
	}
	return nil, errors.Trace(err)
}

func (st *State) processCommonModelApplicationArgs(args *AddApplicationArgs) error {
	if args.Series == "" {
		// args.Series is not set, so use the series in the URL.
		args.Series = args.Charm.URL().Series
		if args.Series == "" {
			// Should not happen, but just in case.
			return errors.New("series is empty")
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
				return errors.Trace(err)
			}
			supportedOperatingSystems := make(map[os.OSType]bool)
			for _, supportedSeries := range supportedSeries {
				os, err := series.GetOSFromSeries(supportedSeries)
				if err != nil {
					// If we can't figure out a series written in the charm
					// just skip it.
					continue
				}
				supportedOperatingSystems[os] = true
			}
			if !supportedOperatingSystems[seriesOS] {
				return errors.NewNotSupported(errors.Errorf(
					"series %q (OS %q) not supported by charm, supported series are %q",
					args.Series, seriesOS, strings.Join(supportedSeries, ", "),
				), "")
			}
		}
	}

	// Ignore constraints that result from this call as
	// these would be accumulation of model and application constraints
	// but we only want application constraints to be persisted here.
	cons, err := st.ResolveConstraints(args.Constraints)
	if err != nil {
		return errors.Trace(err)
	}
	unsupported, err := st.validateConstraints(cons)
	if len(unsupported) > 0 {
		logger.Warningf(
			"deploying %q: unsupported constraints: %v", args.Name, strings.Join(unsupported, ","))
	}
	return errors.Trace(err)
}

func (st *State) processIAASModelApplicationArgs(args *AddApplicationArgs) error {
	if err := st.processCommonModelApplicationArgs(args); err != nil {
		return errors.Trace(err)
	}

	storagePools := make(set.Strings)
	for _, storageParams := range args.Storage {
		storagePools.Add(storageParams.Pool)
	}

	// Obtain volume attachment params corresponding to storage being
	// attached. We need to pass them along to precheckInstance, in
	// case the volumes cannot be attached to a machine with the given
	// placement directive.
	sb, err := NewStorageBackend(st)
	if err != nil {
		return errors.Trace(err)
	}
	volumeAttachments := make([]storage.VolumeAttachmentParams, 0, len(args.AttachStorage))
	for _, storageTag := range args.AttachStorage {
		v, err := sb.StorageInstanceVolume(storageTag)
		if errors.IsNotFound(err) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		volumeInfo, err := v.Info()
		if err != nil {
			// Volume has not been provisioned yet,
			// so it cannot be attached.
			continue
		}
		providerType, _, _, err := poolStorageProvider(sb, volumeInfo.Pool)
		if err != nil {
			return errors.Annotatef(err, "cannot attach %s", names.ReadableString(storageTag))
		}
		storageName, _ := names.StorageName(storageTag.Id())
		volumeAttachments = append(volumeAttachments, storage.VolumeAttachmentParams{
			AttachmentParams: storage.AttachmentParams{
				Provider: providerType,
				ReadOnly: args.Charm.Meta().Storage[storageName].ReadOnly,
			},
			Volume:   v.VolumeTag(),
			VolumeId: volumeInfo.VolumeId,
		})
	}

	// Collect distinct placements that need to be checked.
	for _, placement := range args.Placement {
		data, err := st.parsePlacement(placement)
		if err != nil {
			return errors.Trace(err)
		}
		switch data.placementType() {
		case machinePlacement:
			// Ensure that the machine and charm series match.
			m, err := st.Machine(data.machineId)
			if err != nil {
				return errors.Trace(err)
			}
			subordinate := args.Charm.Meta().Subordinate
			if err := validateUnitMachineAssignment(
				m, args.Series, subordinate, storagePools,
			); err != nil {
				return errors.Annotatef(
					err, "cannot deploy to machine %s", m,
				)
			}
			// This placement directive indicates that we're putting a
			// unit on a pre-existing machine. There's no need to
			// precheck the args since we're not starting an instance.

		case directivePlacement:
			if err := st.precheckInstance(
				args.Series,
				args.Constraints,
				data.directive,
				volumeAttachments,
			); err != nil {
				return errors.Trace(err)
			}
		}
	}
	// We want to check the constraints if there's no placement at all.
	if len(args.Placement) == 0 {
		if err := st.precheckInstance(
			args.Series,
			args.Constraints,
			"",
			volumeAttachments,
		); err != nil {
			return errors.Trace(err)
		}
	}

	return nil
}

func (st *State) processCAASModelApplicationArgs(args *AddApplicationArgs) error {
	if err := st.processCommonModelApplicationArgs(args); err != nil {
		return errors.Trace(err)
	}
	if len(args.Placement) > 0 {
		return errors.NotValidf("placement directives on k8s models")
	}
	return st.precheckInstance(
		args.Series,
		args.Constraints,
		"",
		nil,
	)
}

// removeNils removes any keys with nil values from the given map.
func removeNils(m map[string]interface{}) {
	for k, v := range m {
		if v == nil {
			delete(m, k)
		}
	}
}

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
	col, closer := st.db().GetCollection(assignUnitC)
	defer closer()

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

	data, err := st.parsePlacement(placement)
	if err != nil {
		return errors.Trace(err)
	}
	if data.placementType() == directivePlacement {
		return unit.assignToNewMachine(data.directive)
	}

	m, err := st.addMachineWithPlacement(unit, data)
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

// addMachineWithPlacement finds a machine that matches the given
// placement directive for the given unit.
func (st *State) addMachineWithPlacement(unit *Unit, data *placementData) (*Machine, error) {
	unitCons, err := unit.Constraints()
	if err != nil {
		return nil, err
	}

	// Create any new machine marked as dirty so that
	// nothing else will grab it before we assign the unit to it.
	// TODO(natefinch) fix this when we put assignment in the same
	// transaction as adding a machine.  See bug
	// https://launchpad.net/bugs/1506994

	mId := data.machineId
	var machine *Machine
	if data.machineId != "" {
		machine, err = st.Machine(mId)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Check if an upgrade-series lock is present for the requested
		// machine or its parent.
		// If one exists, return an error to prevent deployment.
		locked, err := machine.IsLockedForSeriesUpgrade()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if locked {
			return nil, errors.Errorf("machine %q is locked for series upgrade", mId)
		}
		locked, err = machine.IsParentLockedForSeriesUpgrade()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if locked {
			return nil, errors.Errorf("machine hosting %q is locked for series upgrade", mId)
		}
	}

	switch data.placementType() {
	case containerPlacement:
		// If a container is to be used, create it.
		template := MachineTemplate{
			Series:      unit.Series(),
			Jobs:        []MachineJob{JobHostUnits},
			Dirty:       true,
			Constraints: *unitCons,
		}
		if mId != "" {
			return st.AddMachineInsideMachine(template, mId, data.containerType)
		}
		return st.AddMachineInsideNewMachine(template, template, data.containerType)
	case directivePlacement:
		return nil, errors.NotSupportedf("programming error: directly adding a machine for %s with a non-machine placement directive", unit.Name())
	default:
		return machine, nil
	}
}

// Application returns a application state by name.
func (st *State) Application(name string) (_ *Application, err error) {
	applications, closer := st.db().GetCollection(applicationsC)
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

// AllApplications returns all deployed applications in the model.
func (st *State) AllApplications() (applications []*Application, err error) {
	applicationsCollection, closer := st.db().GetCollection(applicationsC)
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
// There must be 1 or 2 supplied names, of the form <application>[:<relation>].
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
				scopeOk, err := containerScopeOk(st, ep1, ep2)
				if err != nil {
					return nil, errors.Trace(err)
				}
				if ep1.CanRelateTo(ep2) && scopeOk {
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

func containerScopeOk(st *State, ep1, ep2 Endpoint) (bool, error) {
	if ep1.Scope != charm.ScopeContainer && ep2.Scope != charm.ScopeContainer {
		return true, nil
	}
	var subordinateCount int
	for _, ep := range []Endpoint{ep1, ep2} {
		app, err := applicationByName(st, ep.ApplicationName)
		if err != nil {
			return false, err
		}
		// Container scoped relations are not allowed for remote applications.
		if app.IsRemote() {
			return false, nil
		}
		if app.(*Application).doc.Subordinate {
			subordinateCount++
		}
	}
	return subordinateCount >= 1, nil
}

func applicationByName(st *State, name string) (ApplicationEntity, error) {
	app, err := st.Application(name)
	if err == nil {
		return app, nil
	} else if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	remoteApp, remoteErr := st.RemoteApplication(name)
	if errors.IsNotFound(remoteErr) {
		// We can't find either an application or a remote application
		// by that name. Report the missing application, since that's
		// probably what was intended (and still indicates the problem
		// for remote applications).
		return nil, err
	}
	return remoteApp, remoteErr
}

// endpoints returns all endpoints that could be intended by the
// supplied endpoint name, and which cause the filter param to
// return true.
func (st *State) endpoints(name string, filter func(ep Endpoint) bool) ([]Endpoint, error) {
	var appName, relName string
	if i := strings.Index(name, ":"); i == -1 {
		appName = name
	} else if i != 0 && i != len(name)-1 {
		appName = name[:i]
		relName = name[i+1:]
	} else {
		return nil, errors.Errorf("invalid endpoint %q", name)
	}
	app, err := applicationByName(st, appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	eps := []Endpoint{}
	if relName != "" {
		ep, err := app.Endpoint(relName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		eps = append(eps, ep)
	} else {
		eps, err = app.Endpoints()
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

	// Check applications are alive and do checks if one is remote.
	app1, err := aliveApplication(st, eps[0].ApplicationName)
	if err != nil {
		return nil, err
	}
	app2, err := aliveApplication(st, eps[1].ApplicationName)
	if err != nil {
		return nil, err
	}
	if app1.IsRemote() && app2.IsRemote() {
		return nil, errors.Errorf("cannot add relation between remote applications %q and %q", eps[0].ApplicationName, eps[1].ApplicationName)
	}
	remoteRelation := app1.IsRemote() || app2.IsRemote()
	ep0ok := app1.IsRemote() || eps[0].Scope == charm.ScopeGlobal
	ep1ok := app2.IsRemote() || eps[1].Scope == charm.ScopeGlobal
	if remoteRelation && (!ep0ok || !ep1ok) {
		return nil, errors.Errorf("local endpoint must be globally scoped for remote relations")
	}

	// If either endpoint has container scope, so must the other; and the
	// applications's series must also match, because they'll be deployed to
	// the same machines.
	compatibleSeries := true
	if eps[0].Scope == charm.ScopeContainer {
		eps[1].Scope = charm.ScopeContainer
	} else if eps[1].Scope == charm.ScopeContainer {
		eps[0].Scope = charm.ScopeContainer
	} else {
		compatibleSeries = false
	}
	// We only get a unique relation id once, to save on roundtrips. If it's
	// -1, we haven't got it yet (we don't get it at this stage, because we
	// still don't know whether it's sane to even attempt creation).
	id := -1
	// If a application's charm is upgraded while we're trying to add a relation,
	// we'll need to re-validate application sanity.
	var doc *relationDoc
	now := st.clock().Now()
	buildTxn := func(attempt int) ([]txn.Op, error) {
		// Perform initial relation sanity check.
		if exists, err := isNotDead(st, relationsC, key); err != nil {
			return nil, errors.Trace(err)
		} else if exists {
			return nil, errors.AlreadyExistsf("relation %v", key)
		}
		// Collect per-application operations, checking sanity as we go.
		var ops []txn.Op
		var subordinateCount int
		appSeries := map[string][]string{}
		for _, ep := range eps {
			app, err := aliveApplication(st, ep.ApplicationName)
			if err != nil {
				return nil, err
			}
			if app.IsRemote() {
				// If the remote application is known to be terminated, we don't
				// allow a relation to it.
				statusInfo, err := app.Status()
				if err != nil && !errors.IsNotFound(err) {
					return nil, errors.Trace(err)
				}
				if err == nil && statusInfo.Status == status.Terminated {
					return nil, errors.Errorf("remote offer %s is terminated", ep.ApplicationName)
				}
				ops = append(ops, txn.Op{
					C:      remoteApplicationsC,
					Id:     st.docID(ep.ApplicationName),
					Assert: bson.D{{"life", Alive}},
					Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
				})
			} else {
				localApp := app.(*Application)
				if localApp.doc.Subordinate {
					subordinateCount++
				}
				ch, _, err := localApp.Charm()
				if err != nil {
					return nil, errors.Trace(err)
				}
				if !ep.ImplementedBy(ch) {
					return nil, errors.Errorf("%q does not implement %q", ep.ApplicationName, ep)
				}
				charmSeries := ch.Meta().Series
				if len(charmSeries) == 0 {
					charmSeries = []string{localApp.doc.Series}
				}
				appSeries[localApp.doc.Name] = charmSeries
				ops = append(ops, txn.Op{
					C:      applicationsC,
					Id:     st.docID(ep.ApplicationName),
					Assert: bson.D{{"life", Alive}, {"charmurl", ch.URL()}},
					Update: bson.D{{"$inc", bson.D{{"relationcount", 1}}}},
				})
			}
		}
		if compatibleSeries && len(appSeries) > 1 {
			// We need to ensure that there's intersection between the supported
			// series of both applications' charms.
			app1Series := set.NewStrings(appSeries[eps[0].ApplicationName]...)
			app2Series := set.NewStrings(appSeries[eps[1].ApplicationName]...)
			if app1Series.Intersection(app2Series).Size() == 0 {
				return nil, errors.Errorf("principal and subordinate applications' series must match")
			}
		}
		if eps[0].Scope == charm.ScopeContainer && subordinateCount < 1 {
			return nil, errors.Errorf("container scoped relation requires at least one subordinate application")
		}

		// Create a new unique id if that has not already been done, and add
		// an operation to create the relation document.
		if id == -1 {
			var err error
			if id, err = sequence(st, "relation"); err != nil {
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
		relationStatusDoc := statusDoc{
			Status:    status.Joining,
			ModelUUID: st.ModelUUID(),
			Updated:   now.UnixNano(),
		}
		ops = append(ops, txn.Op{
			C:      relationsC,
			Id:     docID,
			Assert: txn.DocMissing,
			Insert: doc,
		}, createStatusOp(st, relationGlobalScope(id), relationStatusDoc))
		return ops, nil
	}
	if err = st.db().Run(buildTxn); err == nil {
		return &Relation{st, *doc}, nil
	}
	return nil, errors.Trace(err)
}

func aliveApplication(st *State, name string) (ApplicationEntity, error) {
	app, err := applicationByName(st, name)
	if errors.IsNotFound(err) {
		return nil, errors.Errorf("application %q does not exist", name)
	} else if err != nil {
		return nil, errors.Trace(err)
	} else if app.Life() != Alive {
		return nil, errors.Errorf("application %q is not alive", name)
	}
	return app, err
}

// EndpointsRelation returns the existing relation with the given endpoints.
func (st *State) EndpointsRelation(endpoints ...Endpoint) (*Relation, error) {
	return st.KeyRelation(relationKey(endpoints))
}

// KeyRelation returns the existing relation with the given key (which can
// be derived unambiguously from the relation's endpoints).
func (st *State) KeyRelation(key string) (*Relation, error) {
	relations, closer := st.db().GetCollection(relationsC)
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
	relations, closer := st.db().GetCollection(relationsC)
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
	relationsCollection, closer := st.db().GetCollection(relationsC)
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

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (st *State) Report() map[string]interface{} {
	return st.workers.Report()
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
	units, closer := st.db().GetCollection(unitsC)
	defer closer()

	doc := unitDoc{}
	err := units.FindId(name).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("unit %q", name)
	}
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get unit %q", name)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newUnit(st, model.Type(), &doc), nil
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

// UnitsInError returns the units which have an agent status of Error.
func (st *State) UnitsInError() ([]*Unit, error) {
	// First, find the agents in error state.
	agentGlobalKeys, err := getEntityKeysForStatus(st, "u", status.Error)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Extract the unit names.
	unitNames := make([]string, len(agentGlobalKeys))
	for i, key := range agentGlobalKeys {
		// agent key prefix is "u#"
		if !strings.HasPrefix(key, "u#") {
			return nil, errors.NotValidf("unit agent global key %q", key)
		}
		unitNames[i] = key[2:]
	}

	// Query the units with the names of units in error.
	units, closer := st.db().GetCollection(unitsC)
	defer closer()

	var docs []unitDoc
	err = units.Find(bson.D{{"name", bson.D{{"$in", unitNames}}}}).All(&docs)
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	result := make([]*Unit, len(docs))
	for i, doc := range docs {
		result[i] = &Unit{st: st, doc: doc, modelType: model.Type()}
	}
	return result, nil
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
		if _, err = u.AssignToCleanMachine(); errors.Cause(err) != noCleanMachines {
			return errors.Trace(err)
		}
		return u.AssignToNewMachineOrContainer()
	case AssignCleanEmpty:
		if _, err = u.AssignToCleanEmptyMachine(); errors.Cause(err) != noCleanMachines {
			return errors.Trace(err)
		}
		return u.AssignToNewMachineOrContainer()
	case AssignNew:
		return errors.Trace(u.AssignToNewMachine())
	}
	return errors.Errorf("unknown unit assignment policy: %q", policy)
}

type hasStartSync interface {
	StartSync()
}
type hasAdvance interface {
	Advance(time.Duration)
}

// StartSync forces watchers to resynchronize their state with the
// database immediately. This will happen periodically automatically.
// This method is called only from tests.
func (st *State) StartSync() {
	if advanceable, ok := st.clock().(hasAdvance); ok {
		// The amount of time we advance here just needs to be more
		// than 10ms as that is the minimum time the txnwatcher
		// is waiting on, however one second is more human noticeable.
		// The state testing StateSuite type changes the polling interval
		// of the pool's txnwatcher to be one second.
		advanceable.Advance(time.Second)
	}
	if syncable, ok := st.workers.txnLogWatcher().(hasStartSync); ok {
		syncable.StartSync()
	}
	st.workers.pingBatcherWorker().Sync()
	st.workers.presenceWatcher().Sync()
}

// SetAdminMongoPassword sets the administrative password
// to access the state. If the password is non-empty,
// all subsequent attempts to access the state must
// be authorized; otherwise no authorization is required.
func (st *State) SetAdminMongoPassword(password string) error {
	err := mongo.SetAdminMongoPassword(st.session, mongo.AdminUser, password)
	return errors.Trace(err)
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

func (st *State) networkEntityGlobalKeyExists(globalKey string, providerId network.Id) (bool, error) {
	col, closer := st.db().GetCollection(providerIDsC)
	defer closer()

	key := st.networkEntityGlobalKey(globalKey, providerId)
	var doc providerIdDoc
	err := col.FindId(key).One(&doc)

	switch err {
	case nil:
		return true, nil
	case mgo.ErrNotFound:
		return false, nil
	default:
		return false, errors.Annotatef(err, "reading provider ID %q", key)
	}
}

func (st *State) networkEntityGlobalKey(globalKey string, providerId network.Id) string {
	return st.docID(globalKey + ":" + string(providerId))
}

// SetSLA sets the SLA on the current connected model.
func (st *State) SetSLA(level, owner string, credentials []byte) error {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	return model.SetSLA(level, owner, credentials)
}

// SetModelMeterStatus sets the meter status for the current connected model.
func (st *State) SetModelMeterStatus(status, info string) error {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	return model.SetMeterStatus(status, info)
}

// ModelMeterStatus returns the meter status for the current connected model.
func (st *State) ModelMeterStatus() (MeterStatus, error) {
	model, err := st.Model()
	if err != nil {
		return MeterStatus{MeterNotAvailable, ""}, errors.Trace(err)
	}
	return model.MeterStatus(), nil
}

// SLALevel returns the SLA level of the current connected model.
func (st *State) SLALevel() (string, error) {
	model, err := st.Model()
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.SLALevel(), nil
}

// SLACredential returns the SLA credential of the current connected model.
func (st *State) SLACredential() ([]byte, error) {
	model, err := st.Model()
	if err != nil {
		return []byte{}, errors.Trace(err)
	}
	return model.SLACredential(), nil
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
// that it should be obvious if it is ever called from a non-test package.
func (st *State) SetClockForTesting(clock clock.Clock) error {
	// Need to restart the lease workers so they get the new clock.
	// Stop them first so they don't try to use it when we're setting it.
	st.workers.Kill()
	err := st.workers.Wait()
	if err != nil {
		return errors.Trace(err)
	}
	st.stateClock = clock
	if db, ok := st.database.(*database); ok {
		db.clock = clock
	}
	err = st.start(st.controllerTag, nil)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// GlobalClockUpdater returns a new globalclock.Updater using the
// State's *mgo.Session.
func (st *State) GlobalClockUpdater() (coreglobalclock.Updater, error) {
	return globalclock.NewUpdater(globalclock.UpdaterConfig{
		Config: globalclock.Config{
			Mongo:      &environMongo{st},
			Collection: globalClockC,
		},
	})
}

func (st *State) globalClockReader() (*globalclock.Reader, error) {
	return globalclock.NewReader(globalclock.ReaderConfig{
		Config: globalclock.Config{
			Mongo:      &environMongo{st},
			Collection: globalClockC,
		},
	})
}
