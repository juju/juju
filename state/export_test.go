// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time" // Only used for time types.

	"github.com/juju/charm/v12"
	"github.com/juju/clock"
	"github.com/juju/clock/testclock"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn/v3"
	txntesting "github.com/juju/txn/v3/testing"
	jutils "github.com/juju/utils/v3"
	"github.com/juju/worker/v3"
	"github.com/kr/pretty"
	gc "gopkg.in/check.v1"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/testcharms/repo"
	"github.com/juju/juju/version"
)

const (
	MachinesC         = machinesC
	ModelEntityRefsC  = modelEntityRefsC
	ApplicationsC     = applicationsC
	OfferConnectionsC = offerConnectionsC
	EndpointBindingsC = endpointBindingsC
	ControllersC      = controllersC
	UsersC            = usersC
	BlockDevicesC     = blockDevicesC
	StorageInstancesC = storageInstancesC
	GlobalSettingsC   = globalSettingsC
	SettingsC         = settingsC
	UnitsC            = unitsC
	VirtualHostKeysC  = virtualHostKeysC
	SSHConnRequestsC  = sshConnRequestsC
)

var (
	BinarystorageNew              = &binarystorageNew
	MachineIdLessThan             = machineIdLessThan
	CombineMeterStatus            = combineMeterStatus
	ApplicationGlobalKey          = applicationGlobalKey
	CloudGlobalKey                = cloudGlobalKey
	RegionSettingsGlobalKey       = regionSettingsGlobalKey
	ModelGlobalKey                = modelGlobalKey
	DBCollectionSizeToInt         = dbCollectionSizeToInt
	NewEntityWatcher              = newEntityWatcher
	ApplicationHasConnectedOffers = applicationHasConnectedOffers
	NewActionNotificationWatcher  = newActionNotificationWatcher
	SSHReqConnKeyID               = sshReqConnKeyID
)

type (
	CharmDoc       charmDoc
	ApplicationDoc = applicationDoc
	ConstraintsDoc = constraintsDoc

	StorageBackend         = storageBackend
	DeviceBackend          = deviceBackend
	ControllerNodeInstance = controllerNode
)

var (
	IsDying = isDying
)

func NewStateSettingsForCollection(backend modelBackend, collection string) *StateSettings {
	return &StateSettings{backend, globalSettingsC}
}

// EnsureWorkersStarted ensures that all the automatically
// started state workers are running, so that tests which
// insert transaction hooks are less likely to have the hooks
// run by some other worker, and any side effects of starting
// the workers (for example, creating collections) will have
// taken effect.
func EnsureWorkersStarted(st *State) {
	// Note: we don't start the all-watcher workers, as
	// they're started on demand anyway.
	st.workers.txnLogWatcher()
}

func SetTestHooks(c *gc.C, st *State, hooks ...jujutxn.TestHook) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetTestHooks(c, newRunnerForHooks(st), hooks...)
}

func SetBeforeHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetBeforeHooks(c, newRunnerForHooks(st), fs...)
}

// SetFailIfTransaction will set a transaction hook that marks the test as an error
// if there is a transaction run. This is used if you know a given set of operations
// should *not* trigger database updates.
func SetFailIfTransaction(c *gc.C, st *State) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetFailIfTransaction(c, newRunnerForHooks(st))
}

func SetAfterHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetAfterHooks(c, newRunnerForHooks(st), fs...)
}

func SetRetryHooks(c *gc.C, st *State, block, check func()) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetRetryHooks(c, newRunnerForHooks(st), block, check)
}

func SetMaxTxnAttempts(c *gc.C, st *State, n int) {
	st.maxTxnAttempts = n
	db := st.database.(*database)
	db.maxTxnAttempts = n
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{
		Database:                  db.raw,
		Clock:                     st.stateClock,
		TransactionCollectionName: "txns",
		ChangeLogName:             "-",
		ServerSideTransactions:    true,
		MaxRetryAttempts:          db.maxTxnAttempts,
	})
	db.runner = runner
	return
}

func newRunnerForHooks(st *State) jujutxn.Runner {
	db := st.database.(*database)
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{
		Database:                  db.raw,
		Clock:                     st.stateClock,
		TransactionCollectionName: "txns",
		ChangeLogName:             "-",
		ServerSideTransactions:    true,
		RunTransactionObserver: func(t jujutxn.Transaction) {
			txnLogger.Tracef("ran transaction in %.3fs (retries: %d) %# v\nerr: %v",
				t.Duration.Seconds(), t.Attempt, pretty.Formatter(t.Ops), t.Error)
		},
		MaxRetryAttempts: db.maxTxnAttempts,
	})
	db.runner = runner
	return runner
}

// SetPolicy updates the State's policy field to the
// given Policy, and returns the old value.
func SetPolicy(st *State, p Policy) Policy {
	old := st.policy
	st.policy = p
	return old
}

func CloudModelRefCount(st *State, cloudName string) (int, error) {
	refcounts, closer := st.db().GetCollection(globalRefcountsC)
	defer closer()

	key := cloudModelRefCountKey(cloudName)
	return nsRefcounts.read(refcounts, key)
}

func ApplicationSettingsRefCount(st *State, appName string, curl *string) (int, error) {
	refcounts, closer := st.db().GetCollection(refcountsC)
	defer closer()

	key := applicationCharmConfigKey(appName, curl)
	return nsRefcounts.read(refcounts, key)
}

func ApplicationOffersRefCount(st *State, appName string) (int, error) {
	refcounts, closer := st.db().GetCollection(refcountsC)
	defer closer()

	key := applicationOffersRefCountKey(appName)
	return nsRefcounts.read(refcounts, key)
}

func ControllerRefCount(st *State, controllerUUID string) (int, error) {
	refcounts, closer := st.db().GetCollection(globalRefcountsC)
	defer closer()

	key := externalControllerRefCountKey(controllerUUID)
	return nsRefcounts.read(refcounts, key)
}

func IncSecretConsumerRefCount(st *State, uri *secrets.URI, inc int) error {
	refCountCollection, ccloser := st.db().GetCollection(refcountsC)
	defer ccloser()
	incOp, err := nsRefcounts.CreateOrIncRefOp(refCountCollection, uri.ID, inc)
	if err != nil {
		return errors.Trace(err)
	}
	return st.db().RunTransaction([]txn.Op{incOp})
}

func SecretBackendRefCount(st *State, backendID string) (int, error) {
	refcounts, closer := st.db().GetCollection(globalRefcountsC)
	defer closer()

	key := secretBackendRefCountKey(backendID)
	return nsRefcounts.read(refcounts, key)
}

func AddTestingCharm(c *gc.C, st *State, name string) *Charm {
	return addCharm(c, st, "quantal", testcharms.Repo.CharmDir(name))
}

func AddTestingCharmFromRepo(c *gc.C, st *State, name string, repo *repo.CharmRepo) *Charm {
	return addCharm(c, st, "quantal", repo.CharmDir(name))
}

func AddTestingCharmWithSeries(c *gc.C, st *State, name string, series string) *Charm {
	return addCharm(c, st, series, testcharms.Repo.CharmDir(name))
}

func getCharmRepo(series string) *repo.CharmRepo {
	// All testing charms for state are under `quantal` except `kubernetes`.
	if series == "kubernetes" {
		return testcharms.RepoForSeries(series)
	}
	return testcharms.Repo
}

func AddTestingCharmForSeries(c *gc.C, st *State, series, name string) *Charm {
	// Existing logic!
	// Get charm from `quantal` dir or `kubernetes`.
	return addCharm(c, st, series, getCharmRepo(series).CharmDir(name))
}

func AddTestingCharmhubCharmForSeries(c *gc.C, st *State, series, name string) *Charm {
	ch := getCharmRepo(series).CharmDir(name)
	ident := fmt.Sprintf("amd64/%s/%s-%d", series, name, ch.Revision())
	curl := "ch:" + ident
	info := CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      ident + "-sha256",
	}
	sch, err := st.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

func AddTestingCharmMultiSeries(c *gc.C, st *State, name string) *Charm {
	ch := testcharms.Repo.CharmDir(name)
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := "ch:" + ident
	info := CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      ident + "-sha256",
	}
	sch, err := st.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

func AddTestingApplication(c *gc.C, st *State, name string, ch *Charm) *Application {
	return addTestingApplication(c, addTestingApplicationParams{
		st:   st,
		name: name,
		ch:   ch,
	})
}

func AddTestingApplicationForBase(c *gc.C, st *State, base Base, name string, ch *Charm) *Application {
	return addTestingApplication(c, addTestingApplicationParams{
		st: st,
		origin: &CharmOrigin{Platform: &Platform{
			OS:      base.OS,
			Channel: base.Channel,
		}},
		name: name,
		ch:   ch,
	})
}

func AddTestingApplicationWithNumUnits(c *gc.C, st *State, numUnits int, name string, ch *Charm) *Application {
	return addTestingApplication(c, addTestingApplicationParams{
		st:       st,
		numUnits: numUnits,
		name:     name,
		ch:       ch,
	})
}

func AddTestingApplicationWithStorage(c *gc.C, st *State, name string, ch *Charm, storage map[string]StorageConstraints) *Application {
	curl := charm.MustParseURL(ch.URL())
	series := curl.Series
	if series == "kubernetes" {
		series = "focal"
	}
	base, err := corebase.GetBaseFromSeries(series)
	c.Assert(err, jc.ErrorIsNil)
	var source string
	switch curl.Schema {
	case "local":
		source = "local"
	case "ch":
		source = "charm-hub"
	case "cs":
		source = "charm-store"
	}
	origin := &CharmOrigin{
		Source: source,
		Platform: &Platform{
			OS:      base.OS,
			Channel: base.Channel.String(),
		},
	}
	return addTestingApplication(c, addTestingApplicationParams{
		st:      st,
		name:    name,
		ch:      ch,
		origin:  origin,
		storage: storage,
	})
}

func AddTestingApplicationWithDevices(c *gc.C, st *State, name string, ch *Charm, devices map[string]DeviceConstraints) *Application {
	return addTestingApplication(c, addTestingApplicationParams{
		st:      st,
		name:    name,
		ch:      ch,
		devices: devices,
	})
}

func AddTestingApplicationWithBindings(c *gc.C, st *State, name string, ch *Charm, bindings map[string]string) *Application {
	return addTestingApplication(c, addTestingApplicationParams{
		st:       st,
		name:     name,
		ch:       ch,
		bindings: bindings,
	})
}

type addTestingApplicationParams struct {
	st       *State
	name     string
	ch       *Charm
	origin   *CharmOrigin
	bindings map[string]string
	storage  map[string]StorageConstraints
	devices  map[string]DeviceConstraints
	numUnits int
}

func addTestingApplication(c *gc.C, params addTestingApplicationParams) *Application {
	c.Assert(params.ch, gc.NotNil)
	origin := params.origin
	curl := charm.MustParseURL(params.ch.URL())
	if origin == nil {
		base, err := corebase.GetBaseFromSeries(curl.Series)
		c.Assert(err, jc.ErrorIsNil)
		var channel *Channel
		// local charms cannot have a channel
		if charm.CharmHub.Matches(curl.Schema) {
			channel = &Channel{Risk: "stable"}
		}
		var source string
		switch curl.Schema {
		case "local":
			source = "local"
		case "ch":
			source = "charm-hub"
		case "cs":
			source = "charm-store"
		}
		origin = &CharmOrigin{
			Channel: channel,
			Source:  source,
			Platform: &Platform{
				OS:      base.OS,
				Channel: base.Channel.String(),
			},
		}
	}
	app, err := params.st.AddApplication(AddApplicationArgs{
		Name:             params.name,
		Charm:            params.ch,
		CharmOrigin:      origin,
		EndpointBindings: params.bindings,
		Storage:          params.storage,
		Devices:          params.devices,
		NumUnits:         params.numUnits,
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func addCustomCharmWithManifest(c *gc.C, st *State, repo *repo.CharmRepo, name, filename, content, series string, revision int, manifest bool) *Charm {
	path := repo.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		if manifest {
			manifestContent := `
bases:
- name: ubuntu
  channel: "18.04"
`
			manifestYAML := filepath.Join(path, "manifest.yaml")
			err := os.WriteFile(manifestYAML, []byte(manifestContent), 0644)
			c.Assert(err, jc.ErrorIsNil)
		}
		config := filepath.Join(path, filename)
		err := os.WriteFile(config, []byte(content), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	ch, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	if revision != -1 {
		ch.SetRevision(revision)
	}
	return addCharm(c, st, series, ch)
}

func addCustomCharm(c *gc.C, st *State, repo *repo.CharmRepo, name, filename, content, series string, revision int) *Charm {
	return addCustomCharmWithManifest(c, st, repo, name, filename, content, series, revision, false)
}

func AddCustomCharmWithManifest(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	return addCustomCharmWithManifest(c, st, testcharms.RepoForSeries(series), name, filename, content, series, revision, true)
}

func AddCustomCharmForSeries(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	// Copy charm from `series` dir.
	return addCustomCharm(c, st, testcharms.RepoForSeries(series), name, filename, content, series, revision)
}

func AddCustomCharm(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	return addCustomCharm(c, st, getCharmRepo(series), name, filename, content, series, revision)
}

func addCharm(c *gc.C, st *State, series string, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%s-%d", series, ch.Meta().Name, ch.Revision())
	curl := "local:" + series + "/" + ident
	if series == "" {
		ident = fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
		curl = "local:" + ident
	}
	info := CharmInfo{
		Charm:       ch,
		ID:          curl,
		StoragePath: "dummy-path",
		SHA256:      ident + "-sha256",
	}
	sch, err := st.AddCharm(info)
	c.Assert(err, jc.ErrorIsNil)
	return sch
}

// TxnRevno returns the txn-revno field of the document
// associated with the given Id in the given collection.
func TxnRevno(st *State, collName string, id interface{}) (int64, error) {
	var doc struct {
		TxnRevno int64 `bson:"txn-revno"`
	}
	coll, closer := st.db().GetCollection(collName)
	defer closer()
	err := coll.FindId(id).One(&doc)
	if err != nil {
		return 0, err
	}
	return doc.TxnRevno, nil
}

// MinUnitsRevno returns the Revno of the minUnits document
// associated with the given application name.
func MinUnitsRevno(st *State, applicationname string) (int, error) {
	minUnitsCollection, closer := st.db().GetCollection(minUnitsC)
	defer closer()
	var doc minUnitsDoc
	if err := minUnitsCollection.FindId(applicationname).One(&doc); err != nil {
		return 0, err
	}
	return doc.Revno, nil
}

func ConvertTagToCollectionNameAndId(st *State, tag names.Tag) (string, interface{}, error) {
	return st.tagToCollectionAndId(tag)
}

func NowToTheSecond(st *State) time.Time {
	return st.nowToTheSecond()
}

// Return the PasswordSalt that goes along with the PasswordHash
func GetUserPasswordSaltAndHash(u *User) (string, string) {
	return u.doc.PasswordSalt, u.doc.PasswordHash
}

func CheckUserExists(st *State, name string) (bool, error) {
	return st.checkUserExists(name)
}

func WatcherMergeIds(changeset *[]string, updates map[interface{}]bool, idconv func(string) (string, error)) error {
	return mergeIds(changeset, updates, idconv)
}

func WatcherEnsureSuffixFn(marker string) func(string) string {
	return ensureSuffixFn(marker)
}

func WatcherMakeIdFilter(st *State, marker string, receivers ...ActionReceiver) func(interface{}) bool {
	return makeIdFilter(st, marker, receivers...)
}

func GetAllUpgradeInfos(st *State) ([]*UpgradeInfo, error) {
	upgradeInfos, closer := st.db().GetCollection(upgradeInfoC)
	defer closer()

	var out []*UpgradeInfo
	var doc upgradeInfoDoc
	iter := upgradeInfos.Find(nil).Iter()
	defer iter.Close()
	for iter.Next(&doc) {
		out = append(out, &UpgradeInfo{st: st, doc: doc})
	}
	if err := iter.Close(); err != nil {
		return nil, err
	}
	return out, nil
}

func UserModelNameIndex(username, modelName string) string {
	return userModelNameIndex(username, modelName)
}

func (m *Model) UniqueIndexExists() bool {
	coll, closer := m.st.db().GetCollection(usermodelnameC)
	defer closer()

	var doc bson.M
	err := coll.FindId(m.uniqueIndexID()).One(&doc)

	return err == nil
}

func DocID(mb modelBackend, id string) string {
	return mb.docID(id)
}

func LocalID(mb modelBackend, id string) string {
	return mb.localID(id)
}

func StrictLocalID(mb modelBackend, id string) (string, error) {
	return mb.strictLocalID(id)
}

func GetUnitModelUUID(unit *Unit) string {
	return unit.doc.ModelUUID
}

func GetCollection(mb modelBackend, name string) (mongo.Collection, func()) {
	return mb.db().GetCollection(name)
}

func GetRawCollection(mb modelBackend, name string) (*mgo.Collection, func()) {
	return mb.db().GetRawCollection(name)
}

func HasRawAccess(collectionName string) bool {
	return allCollections()[collectionName].rawAccess
}

func MultiModelCollections() []string {
	var result []string
	for name, info := range allCollections() {
		if !info.global {
			result = append(result, name)
		}
	}
	return result
}

func Sequence(st *State, name string) (int, error) {
	return sequence(st, name)
}

func ResetSequence(mb modelBackend, name string) error {
	return resetSequence(mb, name)
}

func SequenceWithMin(st *State, name string, minVal int) (int, error) {
	return sequenceWithMin(st, name, minVal)
}

func SequenceEnsure(st *State, name string, nextVal int) error {
	sequences, closer := st.db().GetRawCollection(sequenceC)
	defer closer()
	updater := newDbSeqUpdater(sequences, st.ModelUUID(), name)
	return updater.ensure(nextVal)
}

func (m *Model) SetDead() error {
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}, {
		C:      usermodelnameC,
		Id:     m.uniqueIndexID(),
		Remove: true,
	}}
	return m.st.db().RunTransaction(ops)
}

func (st *State) SetDyingModelToDead() error {
	return st.setDyingModelToDead()
}

func HostedModelCount(c *gc.C, st *State) int {
	count, err := hostedModelCount(st)
	c.Assert(err, jc.ErrorIsNil)
	return count
}

type MockGlobalEntity struct {
}

func (m MockGlobalEntity) globalKey() string {
	return "globalKey"
}
func (m MockGlobalEntity) Tag() names.Tag {
	return names.NewMachineTag("42")
}

var (
	_                    GlobalEntity = (*MockGlobalEntity)(nil)
	TagToCollectionAndId              = (*State).tagToCollectionAndId
)

func AssertAddressConversion(c *gc.C, netAddr network.SpaceAddress) {
	addr := fromNetworkAddress(netAddr, network.OriginUnknown)
	newNetAddr := addr.networkAddress()
	c.Assert(netAddr, gc.DeepEquals, newNetAddr)

	size := 5
	netAddrs := make(network.SpaceAddresses, size)
	for i := 0; i < size; i++ {
		netAddrs[i] = netAddr
	}
	addrs := fromNetworkAddresses(netAddrs, network.OriginUnknown)
	newNetAddrs := networkAddresses(addrs)
	c.Assert(netAddrs, gc.DeepEquals, newNetAddrs)
}

func AssertHostPortConversion(c *gc.C, netHostPort network.SpaceHostPort) {
	hostPort := fromNetworkHostPort(netHostPort)
	newNetHostPort := hostPort.networkHostPort()
	c.Assert(netHostPort, gc.DeepEquals, newNetHostPort)

	size := 5
	netHostsPorts := make([]network.SpaceHostPorts, size)
	for i := 0; i < size; i++ {
		netHostsPorts[i] = make(network.SpaceHostPorts, size)
		for j := 0; j < size; j++ {
			netHostsPorts[i][j] = netHostPort
		}
	}
	hostsPorts := fromNetworkHostsPorts(netHostsPorts)
	newNetHostsPorts := networkHostsPorts(hostsPorts)
	c.Assert(netHostsPorts, gc.DeepEquals, newNetHostsPorts)
}

// MakeLogDoc creates a database document for a single log message.
func MakeLogDoc(
	entity string,
	t time.Time,
	module string,
	location string,
	level loggo.Level,
	msg string,
	labels []string,
) *logDoc {
	return &logDoc{
		Id:       bson.NewObjectId(),
		Time:     t.UnixNano(),
		Entity:   entity,
		Version:  version.Current.String(),
		Module:   module,
		Location: location,
		Level:    int(level),
		Message:  msg,
		Labels:   labels,
	}
}

func SpaceDoc(s *Space) spaceDoc {
	return s.doc
}

func ForceDestroyMachineOps(m *Machine) ([]txn.Op, error) {
	// For test we do not want to wait for the force.
	return m.forceDestroyOps(time.Duration(0))
}

func MakeActionIdConverter(st *State) func(string) (string, error) {
	return func(id string) (string, error) {
		id, err := st.strictLocalID(id)
		if err != nil {
			return "", errors.Trace(err)
		}
		return actionNotificationIdToActionId(id), err
	}
}

func UpdateModelUserLastConnection(st *State, e permission.UserAccess, when time.Time) error {
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}

	return model.updateLastModelConnection(e.UserTag, when)
}

func SetWantsVote(st *State, id string, wantsVote bool) error {
	op := setControllerWantsVoteOp(st, id, wantsVote)
	return st.runRawTransaction([]txn.Op{op})
}

func RemoveEndpointBindingsForApplication(c *gc.C, app *Application) {
	globalKey := app.globalKey()
	removeOp := removeEndpointBindingsOp(globalKey)

	txnError := app.st.db().RunTransaction([]txn.Op{removeOp})
	err := onAbort(txnError, nil) // ignore ErrAborted as it asserts DocExists
	c.Assert(err, jc.ErrorIsNil)
}

func RemoveOfferConnectionsForRelation(c *gc.C, rel *Relation) {
	removeOps := removeOfferConnectionsForRelationOps(rel.Id())
	txnError := rel.st.db().RunTransaction(removeOps)
	err := onAbort(txnError, nil) // ignore ErrAborted as it asserts DocExists
	c.Assert(err, jc.ErrorIsNil)
}

func RelationCount(app *Application) int {
	return app.doc.RelationCount
}

func AssertEndpointBindingsNotFoundForApplication(c *gc.C, app *Application) {
	globalKey := app.globalKey()
	storedBindings, _, err := readEndpointBindings(app.st, globalKey)
	c.Assert(storedBindings, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("endpoint bindings for %q not found", globalKey))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func StorageAttachmentCount(instance StorageInstance) int {
	internal, ok := instance.(*storageInstance)
	if !ok {
		return -1
	}
	return internal.doc.AttachmentCount
}

func ResetMigrationMode(c *gc.C, st *State) {
	ops := []txn.Op{{
		C:      modelsC,
		Id:     st.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.M{
			"$set": bson.M{"migration-mode": MigrationModeNone},
		},
	}}
	err := st.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func (a *RemoteApplication) SetDead() error {
	ops := []txn.Op{{
		C:      remoteApplicationsC,
		Id:     a.doc.Name,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}}
	return a.st.db().RunTransaction(ops)
}

func RemoveRelationStatus(c *gc.C, rel *Relation) {
	st := rel.st
	ops := []txn.Op{removeStatusOp(st, rel.globalScope())}
	err := st.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func RemoveUnitRelations(c *gc.C, rel *Relation) {
	st := rel.st
	scopes, closer := st.db().GetCollection(relationScopesC)
	defer closer()
	scopesW := scopes.Writeable()
	_, err := scopesW.RemoveAll(nil)
	c.Assert(err, jc.ErrorIsNil)
}

// PrimeUnitStatusHistory will add count history elements, advancing the test clock by
// one second for each entry.
func PrimeUnitStatusHistory(
	c *gc.C, clock testclock.AdvanceableClock,
	unit *Unit, statusVal status.Status,
	count, batchSize int,
	nextData func(int) map[string]interface{},
) {
	globalKey := unit.globalKey()

	history, closer := unit.st.db().GetCollection(statusesHistoryC)
	defer closer()
	historyW := history.Writeable()

	var data map[string]interface{}
	for i := 0; i < count; {
		var docs []interface{}
		for j := 0; j < batchSize && i < count; j++ {
			clock.Advance(time.Second)
			if nextData != nil {
				data = utils.EscapeKeys(nextData(i))
			}
			docs = append(docs, &historicalStatusDoc{
				Status:     statusVal,
				StatusData: data,
				Updated:    clock.Now().UnixNano(),
				GlobalKey:  globalKey,
			})
			// Seems like you can't increment two values in one loop
			i++
		}
		err := historyW.Insert(docs...)
		c.Assert(err, jc.ErrorIsNil)
	}
	// Set the status for the unit itself.
	doc := statusDoc{
		Status:     statusVal,
		StatusData: data,
		Updated:    clock.Now().UnixNano(),
	}

	var buildTxn jujutxn.TransactionSource = func(int) ([]txn.Op, error) {
		return statusSetOps(unit.st.db(), doc, globalKey)
	}

	err := unit.st.db().Run(buildTxn)
	c.Assert(err, jc.ErrorIsNil)
}

// PrimeOperations generates operations and tasks to be pruned.
// The method pads each entry with a 1MB string. This should allow us to infer the
// approximate size of the entry and limit the number of entries that
// must be generated for size related tests.
func PrimeOperations(c *gc.C, age time.Time, unit *Unit, count, actionsPerOperation int) {
	operationsCollection, closer := unit.st.db().GetCollection(operationsC)
	defer closer()
	actionCollection, closer := unit.st.db().GetCollection(actionsC)
	defer closer()

	operationsCollectionWriter := operationsCollection.Writeable()
	actionCollectionWriter := actionCollection.Writeable()

	const numBytes = 1 * 500 * 1000
	var padding [numBytes]byte
	var operationDocs []interface{}
	var actionDocs []interface{}
	for i := 0; i < count; i++ {
		nextID, err := sequenceWithMin(unit.st, "task", 1)
		c.Assert(err, jc.ErrorIsNil)
		operationID := strconv.Itoa(nextID)
		operationDocs = append(operationDocs, operationDoc{
			DocId:     operationID,
			ModelUUID: unit.st.ModelUUID(),
			Summary:   "an operation",
			Completed: age,
			Status:    ActionCompleted,
		})
		for j := 0; j < actionsPerOperation; j++ {
			id, err := jutils.NewUUID()
			c.Assert(err, jc.ErrorIsNil)
			actionDocs = append(actionDocs, actionDoc{
				DocId:     id.String(),
				ModelUUID: unit.st.ModelUUID(),
				Receiver:  unit.Name(),
				Completed: age,
				Operation: operationID,
				Status:    ActionCompleted,
				Message:   string(padding[:numBytes]),
			})
		}
	}

	err := operationsCollectionWriter.Insert(operationDocs...)
	c.Assert(err, jc.ErrorIsNil)
	err = actionCollectionWriter.Insert(actionDocs...)
	c.Assert(err, jc.ErrorIsNil)
}

// PrimeLegacyActions creates actions without a parent operation.
func PrimeLegacyActions(c *gc.C, age time.Time, unit *Unit, count int) {
	actionCollection, closer := unit.st.db().GetCollection(actionsC)
	defer closer()

	actionCollectionWriter := actionCollection.Writeable()

	const numBytes = 1 * 500 * 1000
	var padding [numBytes]byte
	var actionDocs []interface{}
	var ids []string
	for i := 0; i < count; i++ {
		nextID, err := sequenceWithMin(unit.st, "task", 1)
		c.Assert(err, jc.ErrorIsNil)
		ids = append(ids, fmt.Sprintf("%v:%d", unit.st.ModelUUID(), nextID))
		actionDocs = append(actionDocs, actionDoc{
			DocId:     strconv.Itoa(nextID),
			ModelUUID: unit.st.ModelUUID(),
			Receiver:  unit.Name(),
			Completed: age,
			Status:    ActionCompleted,
			Message:   string(padding[:numBytes]),
		})
	}

	err := actionCollectionWriter.Insert(actionDocs...)
	c.Assert(err, jc.ErrorIsNil)
	for _, id := range ids {
		err = actionCollectionWriter.UpdateId(id, bson.D{{"$unset", bson.M{"operation": 1}}})
		c.Assert(err, jc.ErrorIsNil)
	}
}

// ActionOperationId returns the parent operation of an action.
func ActionOperationId(a Action) string {
	return a.(*action).doc.Operation
}

// GetInternalWorkers returns the internal workers managed by a State
// to allow inspection in tests.
func GetInternalWorkers(st *State) worker.Worker {
	return st.workers
}

// ResourceStoragePath returns the path used to store resource content
// in the managed blob store, given the resource ID.
func ResourceStoragePath(c *gc.C, st *State, id string) string {
	p := st.Resources().(*resourcePersistence)
	_, storagePath, err := p.getResource(id)
	c.Assert(err, jc.ErrorIsNil)
	return storagePath
}

func StagedResourceForTest(c *gc.C, st *State, res resources.Resource) *StagedResource {
	persist := st.Resources().(*resourcePersistence)
	storagePath := storagePath(res.Name, res.ApplicationID, res.PendingID)
	r, err := persist.stageResource(res, storagePath)
	c.Assert(err, jc.ErrorIsNil)
	return r
}

// IsBlobStored returns true if a given storage path is in used in the
// managed blob store.
func IsBlobStored(c *gc.C, st *State, storagePath string) bool {
	stor := storage.NewStorage(st.ModelUUID(), st.MongoSession())
	r, _, err := stor.Get(storagePath)
	if err != nil {
		if errors.IsNotFound(err) {
			return false
		}
		c.Fatalf("Get failed: %v", err)
		return false
	}
	r.Close()
	return true
}

// AssertNoCleanupsWithKind checks that there are no cleanups
// of a given kind scheduled.
func AssertNoCleanupsWithKind(c *gc.C, st *State, kind cleanupKind) {
	var docs []cleanupDoc
	cleanups, closer := st.db().GetCollection(cleanupsC)
	defer closer()
	err := cleanups.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	for _, doc := range docs {
		if doc.Kind == kind {
			c.Fatalf("found cleanup of kind %q", kind)
		}
	}
}

// AssertNoCleanupsWithKind checks that there is at least
// one cleanup of a given kind scheduled.
func AssertCleanupsWithKind(c *gc.C, st *State, kind cleanupKind) {
	var docs []cleanupDoc
	cleanups, closer := st.db().GetCollection(cleanupsC)
	defer closer()
	err := cleanups.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	for _, doc := range docs {
		if doc.Kind == kind {
			return
		}
	}
	c.Fatalf("found no cleanups of kind %q", kind)
}

// AssertNoCleanups checks that there are no cleanups scheduled.
func AssertNoCleanups(c *gc.C, st *State) {
	var docs []cleanupDoc
	cleanups, closer := st.db().GetCollection(cleanupsC)
	defer closer()
	err := cleanups.Find(nil).All(&docs)
	c.Assert(err, jc.ErrorIsNil)
	if len(docs) > 0 {
		c.Fatalf("unexpected cleanups: %+v", docs)
	}
}

// GetApplicationCharmConfig allows access to settings collection for a
// given application in order to get the charm config.
func GetApplicationCharmConfig(st *State, app *Application) *Settings {
	return newSettings(st.db(), settingsC, app.charmConfigKey())
}

// GetApplicationConfig allows access to settings collection for a
// given application in order to get the application config.
func GetApplicationConfig(st *State, app *Application) *Settings {
	return newSettings(st.db(), settingsC, app.applicationConfigKey())
}

// GetApplicationHasResources returns the app's HasResources value.
func GetApplicationHasResources(app *Application) bool {
	return app.doc.HasResources
}

// GetControllerSettings allows access to settings collection for
// the controller.
func GetControllerSettings(st *State) *Settings {
	return newSettings(st.db(), controllersC, ControllerSettingsGlobalKey)
}

// GetPopulatedSettings returns a reference to settings with the input values
// populated. Attempting to read/write will cause nil-reference panics.
func GetPopulatedSettings(cfg map[string]interface{}) *Settings {
	return &Settings{
		disk: copyMap(cfg, nil),
		core: copyMap(cfg, nil),
	}
}

// NewSLALevel returns a new SLA level.
func NewSLALevel(level string) (slaLevel, error) {
	return newSLALevel(level)
}

func AppStorageConstraints(app *Application) (map[string]StorageConstraints, error) {
	return readStorageConstraints(app.st, app.storageConstraintsKey())
}

func RemoveRelation(c *gc.C, rel *Relation, force bool) {
	op := &ForcedOperation{Force: force}
	ops, err := rel.removeOps("", "", op)
	c.Assert(err, jc.ErrorIsNil)
	c.Logf("operational errors %v", op.Errors)
	c.Assert(op.Errors, gc.HasLen, 0)
	err = rel.st.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func AddVolumeOps(st *State, params VolumeParams, machineId string) ([]txn.Op, names.VolumeTag, error) {
	sb, err := NewStorageBackend(st)
	if err != nil {
		return nil, names.VolumeTag{}, err
	}
	return sb.addVolumeOps(params, machineId)
}

func ModelBackendFromStorageBackend(sb *StorageBackend) modelBackend {
	return sb.mb
}

func (st *State) ModelQueryForUser(user names.UserTag, isSuperuser bool) (mongo.Query, SessionCloser, error) {
	return st.modelQueryForUser(user, isSuperuser)
}

func UnitsHaveChanged(m *Machine, unitNames []string) (bool, error) {
	return m.unitsHaveChanged(unitNames)
}

func GetCloudContainerStatus(st *State, name string) (status.StatusInfo, error) {
	return getStatus(st.db(), globalCloudContainerKey(name), "unit")
}

func GetCloudContainerStatusHistory(st *State, name string, filter status.StatusHistoryFilter) ([]status.StatusInfo, error) {
	args := &statusHistoryArgs{
		db:        st.db(),
		globalKey: globalCloudContainerKey(name),
		filter:    filter,
		clock:     st.clock(),
	}
	return statusHistory(args)
}

func ApplicationOperatorStatus(st *State, appName string) (status.StatusInfo, error) {
	return getStatus(st.db(), applicationGlobalOperatorKey(appName), "operator")
}

func NewInstanceCharmProfileDataCompatibilityWatcher(backend ModelBackendShim, memberId string) StringsWatcher {
	return watchInstanceCharmProfileCompatibilityData(backend, memberId)
}

func UnitBranch(m *Model, unitName string) (*Generation, error) {
	return m.unitBranch(unitName)
}

func ApplicationBranches(m *Model, appName string) ([]*Generation, error) {
	return m.applicationBranches(appName)
}

func MachinePortOps(st *State, m description.Machine) ([]txn.Op, error) {
	resolver := &importer{st: st}
	return []txn.Op{resolver.machinePortsOp(m)}, nil
}

func ApplicationPortOps(st *State, a description.Application) ([]txn.Op, error) {
	resolver := &importer{st: st}
	return []txn.Op{resolver.applicationPortsOp(a)}, nil
}

func GetSecretNextRotateTime(c *gc.C, st *State, id string) time.Time {
	secretRotateCollection, closer := st.db().GetCollection(secretRotateC)
	defer closer()

	var doc secretRotationDoc
	err := secretRotateCollection.FindId(id).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	return doc.NextRotateTime.UTC()
}

func GetSecretBackendNextRotateInfo(c *gc.C, st *State, id string) (string, time.Time) {
	secretBackendRotateCollection, closer := st.db().GetCollection(secretBackendsRotateC)
	defer closer()

	var doc secretBackendRotationDoc
	err := secretBackendRotateCollection.FindId(id).One(&doc)
	c.Assert(err, jc.ErrorIsNil)
	return doc.Name, doc.NextRotateTime.UTC()
}

// ModelBackendShim is required to live here in the export_test.go file because
// there is issues placing this in the test files themselves. The strangeness
// exhibits itself from the fact that `clock() clock.Clock` doesn't type
// check correctly and the go compiler thinks it should be
// `state.clock() clock.Clock`, which makes no sense.
type ModelBackendShim struct {
	Clock    clock.Clock
	Database Database
	Watcher  watcher.BaseWatcher
}

func (s ModelBackendShim) docID(id string) string {
	return ""
}

func (s ModelBackendShim) localID(id string) string {
	return ""
}

func (s ModelBackendShim) strictLocalID(id string) (string, error) {
	return "", nil
}

func (s ModelBackendShim) nowToTheSecond() time.Time {
	return s.Clock.Now().Round(time.Second).UTC()
}

func (s ModelBackendShim) clock() clock.Clock {
	return s.Clock
}

func (s ModelBackendShim) db() Database {
	return s.Database
}

func (s ModelBackendShim) ModelUUID() string {
	return ""
}

func (s ModelBackendShim) modelName() (string, error) {
	return "", nil
}

func (s ModelBackendShim) IsController() bool {
	return false
}

func (s ModelBackendShim) txnLogWatcher() watcher.BaseWatcher {
	return s.Watcher
}

// SetClockForTesting is an exported function to allow tests
// to set the internal clock for the State instance. It is named such
// that it should be obvious if it is ever called from a non-test package.
// TODO (thumper): This is a terrible method and we should remove it.
// NOTE: this should almost never be needed.
func (st *State) SetClockForTesting(clock clock.Clock) error {
	// Need to restart the lease workers so they get the new clock.
	// Stop them first so they don't try to use it when we're setting it.
	hub := st.workers.hub
	st.workers.Kill()
	err := st.workers.Wait()
	if err != nil {
		return errors.Trace(err)
	}
	st.stateClock = clock
	if db, ok := st.database.(*database); ok {
		db.clock = clock
	}
	err = st.startWorkers(hub)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

var (
	CleanupForceDestroyedUnit = cleanupForceDestroyedUnit
	CleanupForceRemoveUnit    = cleanupForceRemoveUnit
	CleanupForceApplication   = cleanupForceApplication
)

func (st *State) ScheduleForceCleanup(kind cleanupKind, name string, maxWait time.Duration) {
	st.scheduleForceCleanup(kind, name, maxWait)
}

func GetCollectionCappedInfo(coll *mgo.Collection) (bool, int, error) {
	return getCollectionCappedInfo(coll)
}

func (m *Model) AllActionIDsHasActionNotifications() ([]string, error) {
	actionNotifications, closer := m.st.db().GetCollection(actionNotificationsC)
	defer closer()

	docs := []actionNotificationDoc{}
	err := actionNotifications.Find(nil).All(&docs)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot get all actions")
	}
	actionIDs := make([]string, len(docs))
	for i, doc := range docs {
		actionIDs[i] = doc.ActionID
	}
	return actionIDs, nil
}

func AddVirtualHostKey(c *gc.C, st *State, tag names.Tag, key []byte) {
	var docID string
	switch tag.Kind() {
	case names.UnitTagKind:
		docID = unitHostKeyID(tag.Id())
	case names.MachineTagKind:
		docID = machineHostKeyID(tag.Id())
	default:
		c.Fatalf("unsupported tag kind %q for creating a virtual host key", tag.Kind())
	}
	doc := virtualHostKeyDoc{
		DocId:   st.docID(docID),
		HostKey: key,
	}
	op := []txn.Op{{
		C:      virtualHostKeysC,
		Id:     docID,
		Insert: &doc,
		Assert: txn.DocMissing,
	}}
	err := st.db().RunTransaction(op)
	c.Assert(err, gc.IsNil)
}
func RemoveVirtualHostKey(c *gc.C, st *State, key *VirtualHostKey) {
	op := []txn.Op{{
		C:      virtualHostKeysC,
		Id:     key.doc.DocId,
		Remove: true,
		Assert: txn.DocExists,
	}}
	err := st.db().RunTransaction(op)
	c.Assert(err, gc.IsNil)
}
