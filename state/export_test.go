// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"time" // Only used for time types.

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	jujutxn "github.com/juju/txn"
	txntesting "github.com/juju/txn/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	worker "gopkg.in/juju/worker.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/mongo/utils"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state/storage"
	"github.com/juju/juju/status"
	"github.com/juju/juju/testcharms"
	"github.com/juju/juju/version"
)

const (
	MachinesC         = machinesC
	ApplicationsC     = applicationsC
	EndpointBindingsC = endpointBindingsC
	ControllersC      = controllersC
	UsersC            = usersC
	BlockDevicesC     = blockDevicesC
	StorageInstancesC = storageInstancesC
	GUISettingsC      = guisettingsC
	GlobalSettingsC   = globalSettingsC
	SettingsC         = settingsC
)

var (
	BinarystorageNew                     = &binarystorageNew
	ImageStorageNewStorage               = &imageStorageNewStorage
	MachineIdLessThan                    = machineIdLessThan
	ControllerAvailable                  = &controllerAvailable
	GetOrCreatePorts                     = getOrCreatePorts
	GetPorts                             = getPorts
	CombineMeterStatus                   = combineMeterStatus
	ApplicationGlobalKey                 = applicationGlobalKey
	ControllerInheritedSettingsGlobalKey = controllerInheritedSettingsGlobalKey
	ModelGlobalKey                       = modelGlobalKey
	MergeBindings                        = mergeBindings
	UpgradeInProgressError               = errUpgradeInProgress
)

type (
	CharmDoc        charmDoc
	MachineDoc      machineDoc
	RelationDoc     relationDoc
	ApplicationDoc  applicationDoc
	UnitDoc         unitDoc
	BlockDevicesDoc blockDevicesDoc
)

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
	st.workers.presenceWatcher()
	st.workers.leadershipManager()
	st.workers.singularManager()
}

func SetTestHooks(c *gc.C, st *State, hooks ...jujutxn.TestHook) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetTestHooks(c, newRunnerForHooks(st), hooks...)
}

func SetBeforeHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetBeforeHooks(c, newRunnerForHooks(st), fs...)
}

func SetAfterHooks(c *gc.C, st *State, fs ...func()) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetAfterHooks(c, newRunnerForHooks(st), fs...)
}

func SetRetryHooks(c *gc.C, st *State, block, check func()) txntesting.TransactionChecker {
	EnsureWorkersStarted(st)
	return txntesting.SetRetryHooks(c, newRunnerForHooks(st), block, check)
}

func newRunnerForHooks(st *State) jujutxn.Runner {
	db := st.database.(*database)
	runner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db.raw})
	db.runner = runner
	return runner
}

func OfferForName(sd crossmodel.ApplicationOffers, name string) (*applicationOfferDoc, error) {
	return sd.(*applicationOffers).offerForName(name)
}

func MakeApplicationOffer(sd crossmodel.ApplicationOffers, offer *applicationOfferDoc) (*crossmodel.ApplicationOffer, error) {
	return sd.(*applicationOffers).makeApplicationOffer(*offer)
}

// SetPolicy updates the State's policy field to the
// given Policy, and returns the old value.
func SetPolicy(st *State, p Policy) Policy {
	old := st.policy
	st.policy = p
	return old
}

func (doc *MachineDoc) String() string {
	m := &Machine{doc: machineDoc(*doc)}
	return m.String()
}

func ServiceSettingsRefCount(st *State, appName string, curl *charm.URL) (int, error) {
	refcounts, closer := st.db().GetCollection(refcountsC)
	defer closer()

	key := applicationSettingsKey(appName, curl)
	return nsRefcounts.read(refcounts, key)
}

func AddTestingCharm(c *gc.C, st *State, name string) *Charm {
	return addCharm(c, st, "quantal", testcharms.Repo.CharmDir(name))
}

func AddTestingCharmForSeries(c *gc.C, st *State, series, name string) *Charm {
	return addCharm(c, st, series, testcharms.Repo.CharmDir(name))
}

func AddTestingCharmMultiSeries(c *gc.C, st *State, name string) *Charm {
	ch := testcharms.Repo.CharmDir(name)
	ident := fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
	curl := charm.MustParseURL("cs:" + ident)
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

func AddTestingService(c *gc.C, st *State, name string, ch *Charm) *Application {
	return addTestingService(c, st, "", name, ch, nil, nil)
}

func AddTestingServiceForSeries(c *gc.C, st *State, series, name string, ch *Charm) *Application {
	return addTestingService(c, st, series, name, ch, nil, nil)
}

func AddTestingServiceWithStorage(c *gc.C, st *State, name string, ch *Charm, storage map[string]StorageConstraints) *Application {
	return addTestingService(c, st, "", name, ch, nil, storage)
}

func AddTestingServiceWithBindings(c *gc.C, st *State, name string, ch *Charm, bindings map[string]string) *Application {
	return addTestingService(c, st, "", name, ch, bindings, nil)
}

func addTestingService(c *gc.C, st *State, series, name string, ch *Charm, bindings map[string]string, storage map[string]StorageConstraints) *Application {
	c.Assert(ch, gc.NotNil)
	app, err := st.AddApplication(AddApplicationArgs{
		Name:             name,
		Series:           series,
		Charm:            ch,
		EndpointBindings: bindings,
		Storage:          storage,
	})
	c.Assert(err, jc.ErrorIsNil)
	return app
}

func AddCustomCharm(c *gc.C, st *State, name, filename, content, series string, revision int) *Charm {
	path := testcharms.Repo.ClonedDirPath(c.MkDir(), name)
	if filename != "" {
		config := filepath.Join(path, filename)
		err := ioutil.WriteFile(config, []byte(content), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
	ch, err := charm.ReadCharmDir(path)
	c.Assert(err, jc.ErrorIsNil)
	if revision != -1 {
		ch.SetRevision(revision)
	}
	return addCharm(c, st, series, ch)
}

func addCharm(c *gc.C, st *State, series string, ch charm.Charm) *Charm {
	ident := fmt.Sprintf("%s-%s-%d", series, ch.Meta().Name, ch.Revision())
	url := "local:" + series + "/" + ident
	if series == "" {
		ident = fmt.Sprintf("%s-%d", ch.Meta().Name, ch.Revision())
		url = "local:" + ident
	}
	curl := charm.MustParseURL(url)
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

func init() {
	txnLogSize = txnLogSizeTests
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

func RunTransaction(st *State, ops []txn.Op) error {
	return st.db().RunTransaction(ops)
}

// Return the PasswordSalt that goes along with the PasswordHash
func GetUserPasswordSaltAndHash(u *User) (string, string) {
	return u.doc.PasswordSalt, u.doc.PasswordHash
}

func CheckUserExists(st *State, name string) (bool, error) {
	return st.checkUserExists(name)
}

func WatcherMergeIds(st *State, changeset *[]string, updates map[interface{}]bool, idconv func(string) (string, error)) error {
	return mergeIds(st, changeset, updates, idconv)
}

func WatcherEnsureSuffixFn(marker string) func(string) string {
	return ensureSuffixFn(marker)
}

func WatcherMakeIdFilter(st *State, marker string, receivers ...ActionReceiver) func(interface{}) bool {
	return makeIdFilter(st, marker, receivers...)
}

func NewActionStatusWatcher(st *State, receivers []ActionReceiver, statuses ...ActionStatus) StringsWatcher {
	return newActionStatusWatcher(st, receivers, statuses...)
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
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func UserModelNameIndex(username, modelName string) string {
	return userModelNameIndex(username, modelName)
}

func DocID(st *State, id string) string {
	return st.docID(id)
}

func LocalID(st *State, id string) string {
	return st.localID(id)
}

func StrictLocalID(st *State, id string) (string, error) {
	return st.strictLocalID(id)
}

func GetUnitModelUUID(unit *Unit) string {
	return unit.doc.ModelUUID
}

func GetCollection(st *State, name string) (mongo.Collection, func()) {
	return st.db().GetCollection(name)
}

func GetRawCollection(st *State, name string) (*mgo.Collection, func()) {
	return st.db().GetRawCollection(name)
}

func HasRawAccess(collectionName string) bool {
	return allCollections()[collectionName].rawAccess
}

func MultiEnvCollections() []string {
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

func SequenceWithMin(st *State, name string, minVal int) (int, error) {
	return sequenceWithMin(st, name, minVal)
}

func SequenceEnsure(st *State, name string, nextVal int) error {
	sequences, closer := st.db().GetRawCollection(sequenceC)
	defer closer()
	updater := newDbSeqUpdater(sequences, st.ModelUUID(), name)
	return updater.ensure(nextVal)
}

func SetModelLifeDead(st *State, modelUUID string) error {
	ops := []txn.Op{{
		C:      modelsC,
		Id:     modelUUID,
		Update: bson.D{{"$set", bson.D{{"life", Dead}}}},
	}}
	return st.db().RunTransaction(ops)
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

func AssertAddressConversion(c *gc.C, netAddr network.Address) {
	addr := fromNetworkAddress(netAddr, OriginUnknown)
	newNetAddr := addr.networkAddress()
	c.Assert(netAddr, gc.DeepEquals, newNetAddr)

	size := 5
	netAddrs := make([]network.Address, size)
	for i := 0; i < size; i++ {
		netAddrs[i] = netAddr
	}
	addrs := fromNetworkAddresses(netAddrs, OriginUnknown)
	newNetAddrs := networkAddresses(addrs)
	c.Assert(netAddrs, gc.DeepEquals, newNetAddrs)
}

func AssertHostPortConversion(c *gc.C, netHostPort network.HostPort) {
	hostPort := fromNetworkHostPort(netHostPort)
	newNetHostPort := hostPort.networkHostPort()
	c.Assert(netHostPort, gc.DeepEquals, newNetHostPort)

	size := 5
	netHostsPorts := make([][]network.HostPort, size)
	for i := 0; i < size; i++ {
		netHostsPorts[i] = make([]network.HostPort, size)
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
	entity names.Tag,
	t time.Time,
	module string,
	location string,
	level loggo.Level,
	msg string,
) *logDoc {
	return &logDoc{
		Id:       bson.NewObjectId(),
		Time:     t.UnixNano(),
		Entity:   entity.String(),
		Version:  version.Current.String(),
		Module:   module,
		Location: location,
		Level:    int(level),
		Message:  msg,
	}
}

func SpaceDoc(s *Space) spaceDoc {
	return s.doc
}

func ForceDestroyMachineOps(m *Machine) ([]txn.Op, error) {
	return m.forceDestroyOps()
}

func IsManagerMachineError(err error) bool {
	return errors.Cause(err) == managerMachineError
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
	return st.updateLastModelConnection(e.UserTag, when)
}

func RemoveEndpointBindingsForService(c *gc.C, app *Application) {
	globalKey := app.globalKey()
	removeOp := removeEndpointBindingsOp(globalKey)

	txnError := app.st.db().RunTransaction([]txn.Op{removeOp})
	err := onAbort(txnError, nil) // ignore ErrAborted as it asserts DocExists
	c.Assert(err, jc.ErrorIsNil)
}

func RelationCount(app *Application) int {
	return app.doc.RelationCount
}

func AssertEndpointBindingsNotFoundForService(c *gc.C, app *Application) {
	globalKey := app.globalKey()
	storedBindings, _, err := readEndpointBindings(app.st, globalKey)
	c.Assert(storedBindings, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("endpoint bindings for %q not found", globalKey))
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func LeadershipLeases(st *State) (map[string]lease.Info, error) {
	client, err := st.getLeadershipLeaseClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client.Leases(), nil
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

// PrimeUnitStatusHistory will add count history elements, advancing the test clock by
// one second for each entry.
func PrimeUnitStatusHistory(
	c *gc.C, clock *testing.Clock,
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

// GetInternalWorkers returns the internal workers managed by a State
// to allow inspection in tests.
func GetInternalWorkers(st *State) worker.Worker {
	return st.workers
}

// ResourceStoragePath returns the path used to store resource content
// in the managed blob store, given the resource ID.
func ResourceStoragePath(c *gc.C, st *State, id string) string {
	p := NewResourcePersistence(st.newPersistence())
	_, storagePath, err := p.GetResource(id)
	c.Assert(err, jc.ErrorIsNil)
	return storagePath
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

// GetApplicationSettings allows access to settings collection for a
// given application.
func GetApplicationSettings(st *State, app *Application) *Settings {
	return newSettings(st.db(), settingsC, app.settingsKey())
}

// GetControllerSettings allows access to settings collection for
// the controller.
func GetControllerSettings(st *State) *Settings {
	return newSettings(st.db(), controllersC, controllerSettingsGlobalKey)
}

// NewSLALevel returns a new SLA level.
func NewSLALevel(level string) (slaLevel, error) {
	return newSLALevel(level)
}

func AppStorageConstraints(app *Application) (map[string]StorageConstraints, error) {
	return readStorageConstraints(app.st, app.storageConstraintsKey())
}

func RemoveRelation(c *gc.C, rel *Relation) {
	ops, err := rel.removeOps("", "")
	c.Assert(err, jc.ErrorIsNil)
	err = rel.st.db().RunTransaction(ops)
	c.Assert(err, jc.ErrorIsNil)
}

func IngressNetworks(rel *Relation) ([]string, error) {
	relIngress := NewRelationIngressNetworks(rel.st)
	doc, err := relIngress.ingressNetworks(rel.Tag().Id())
	if err != nil {
		return nil, err
	}
	return doc.CIDRS, nil
}

func AddVolumeOps(st *State, params VolumeParams, machineId string) ([]txn.Op, names.VolumeTag, error) {
	im, err := st.IAASModel()
	if err != nil {
		return nil, names.VolumeTag{}, err
	}
	return im.addVolumeOps(params, machineId)
}
