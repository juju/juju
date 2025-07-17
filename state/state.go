// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/pubsub/v2"

	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/semversion"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/state/watcher"
)

var logger = internallogger.GetLogger("juju.state")

const (
	// jujuDB is the name of the main juju database.
	jujuDB = "juju"
)

// State represents the state of an model
// managed by juju.
type State struct {
	stateClock         clock.Clock
	modelTag           names.ModelTag
	controllerModelTag names.ModelTag
	controllerTag      names.ControllerTag
	session            *mgo.Session
	database           Database
	policy             Policy
	newPolicy          NewPolicyFunc
	maxTxnAttempts     int
	// Note(nvinuesa): Having a dqlite domain service here is an awful hack
	// and should disapear as soon as we migrate units and applications.
	charmServiceGetter func(modelUUID coremodel.UUID) (CharmService, error)

	// workers is responsible for keeping the various sub-workers
	// available by starting new ones as they fail. It doesn't do
	// that yet, but having a type that collects them together is the
	// first step.
	workers *workers
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

// ControllerTag returns the tag form of the ControllerUUID.
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
		return errors.Wrap(err, errors.New("can't remove model: model not dead"))
	}
	return errors.Trace(err)
}

func (st *State) removeAllModelDocs(modelAssertion bson.D) error {
	// TODO(secrets) - fix when ref counts are done.
	//if err := cleanupSecretBackendRefCountAfterModelMigrationDone(st); err != nil {
	//	// We have to do this before secrets get removed.
	//	return errors.Trace(err)
	//}

	// Remove each collection in its own transaction.
	modelUUID := st.ModelUUID()
	for name, info := range st.database.Schema() {
		if info.global || info.rawAccess {
			continue
		}

		ops, err := st.removeAllInCollectionOps(name)
		if err != nil {
			return errors.Trace(err)
		}
		if len(ops) == 0 {
			// Nothing to delete.
			continue
		}
		// Make sure we gate everything on the model assertion.
		ops = append([]txn.Op{{
			C:      modelsC,
			Id:     modelUUID,
			Assert: modelAssertion,
		}}, ops...)
		err = st.db().RunTransaction(ops)
		if err != nil {
			return errors.Annotatef(err, "removing from collection %q", name)
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

	// Now remove the model.
	model, err := st.Model()
	if err != nil {
		return errors.Trace(err)
	}
	ops := []txn.Op{{
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

	if !st.IsController() {
		ops = append(ops, decHostedModelCountOp())
	}
	return errors.Trace(st.db().RunTransaction(ops))
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

// startWorkers starts the worker backends on the *State
//   - txn log watcher
//   - txn log pruner
//
// startWorkers will close the *State if it fails.
func (st *State) startWorkers(hub *pubsub.SimpleHub) (err error) {
	defer func() {
		if err == nil {
			return
		}
		if err2 := st.Close(); err2 != nil {
			logger.Errorf(context.TODO(), "closing State for %s: %v", st.modelTag, err2)
		}
	}()

	logger.Infof(context.TODO(), "starting standard state workers")
	workers, err := newWorkers(st, hub)
	if err != nil {
		return errors.Trace(err)
	}
	st.workers = workers
	logger.Infof(context.TODO(), "started state workers for %s successfully", st.modelTag)
	return nil
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

		if err := func(name string, info CollectionInfo) error {
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
			return nil
		}(name, info); err != nil {
			return errors.Trace(err)
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

// Upgrader is an interface that can be used to check if an upgrade is in
// progress.
type Upgrader interface {
	IsUpgrading() (bool, error)
}

// SetModelAgentVersion changes the agent version for the model to the
// given version, only if the model is in a stable state (all agents are
// running the current version). If this is a hosted model, newVersion
// cannot be higher than the controller version.
func (st *State) SetModelAgentVersion(newVersion semversion.Number, stream *string, ignoreAgentVersions bool, upgrader Upgrader) (err error) {
	// TODO - implement the equivalent in the ModelAgentService.
	// Removed as state not longer contains model config. Therefore
	// the modelGlobalKey will never be found in the settingsC again.
	return nil
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
		// Work out what's still calling machine.
		// debug.PrintStack()
		return nil, errors.NotFoundf("machine %s", id)
	default:
		return nil, errors.Annotatef(err, "cannot get machine %s", id)
	}
}

var (
	errLocalApplicationExists = errors.Errorf("application already exists")
)

// SaveCloudServiceArgs defines the arguments for SaveCloudService method.
type SaveCloudServiceArgs struct {
	// Id will be the application Name if it's a part of application,
	// and will be controller UUID for k8s a controller(controller does not have an application),
	// then is wrapped with applicationGlobalKey.
	Id         string
	ProviderId string
	Addresses  network.SpaceAddresses

	Generation            int64
	DesiredScaleProtected bool
}

// CharmRef is an indirection to a charm, this allows us to pass in a charm,
// without having a full concrete charm.
type CharmRef interface {
	Meta() *charm.Meta
	Manifest() *charm.Manifest
}

// CharmRefFull is actually almost a full charm with addition information. This
// is purely here as a hack to push a charm from the dqlite layer to the state
// layer.
// Deprecated: This is an abomination and should be removed.
type CharmRefFull interface {
	CharmRef

	Actions() *charm.Actions
	Config() *charm.Config
	Revision() int
	URL() string
	Version() string
}

// AddApplicationArgs defines the arguments for AddApplication method.
type AddApplicationArgs struct {
	Name              string
	Charm             CharmRef
	CharmURL          string
	CharmOrigin       *CharmOrigin
	Storage           map[string]StorageConstraints
	AttachStorage     []names.StorageTag
	EndpointBindings  map[string]string
	ApplicationConfig *config.Config
	CharmConfig       charm.Settings
	NumUnits          int
	Placement         []*instance.Placement
	Constraints       constraints.Value
	Resources         map[string]string
}

// AddApplication creates a new application, running the supplied charm, with the
// supplied name (which must be unique). If the charm defines peer relations,
// they will be created automatically.
func (st *State) AddApplication(
	args AddApplicationArgs,
	store objectstore.ObjectStore,
) (_ *Application, err error) {
	defer errors.DeferredAnnotatef(&err, "cannot add application %q", args.Name)

	// Sanity checks.
	if !names.IsValidApplication(args.Name) {
		return nil, errors.Errorf("invalid name")
	}
	if args.CharmOrigin == nil {
		return nil, errors.Errorf("charm origin is nil")
	}
	if args.CharmOrigin.Platform == nil {
		return nil, errors.Errorf("charm origin platform is nil")
	}

	// If either the charm origin ID or Hash is set before a charm is
	// downloaded, charm download will fail for charms with a forced series.
	// The logic (refreshConfig) in sending the correct request to charmhub
	// will break.
	if (args.CharmOrigin.ID != "" && args.CharmOrigin.Hash == "") ||
		(args.CharmOrigin.ID == "" && args.CharmOrigin.Hash != "") {
		return nil, errors.BadRequestf("programming error, AddApplication, neither CharmOrigin ID nor Hash can be set before a charm is downloaded. See CharmHubRepository GetDownloadURL.")
	}

	model, err := st.Model()
	if err != nil {
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
				return nil, errors.NotSupportedf("block storage on a container model")
			}
		}
	}

	if len(args.AttachStorage) > 0 && args.NumUnits != 1 {
		return nil, errors.Errorf("AttachStorage is non-empty but NumUnits is %d, must be 1", args.NumUnits)
	}

	if err := jujuversion.CheckJujuMinVersion(args.Charm.Meta().MinJujuVersion, jujuversion.Current); err != nil {
		return nil, errors.Trace(err)
	}

	if exists, err := isNotDead(st, applicationsC, args.Name); err != nil {
		return nil, errors.Trace(err)
	} else if exists {
		return nil, errors.AlreadyExistsf("application")
	}
	if err := checkModelActive(st); err != nil {
		return nil, errors.Trace(err)
	}

	// ensure storage
	if args.Storage == nil {
		args.Storage = make(map[string]StorageConstraints)
	}
	sb, err := NewStorageConfigBackend(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := addDefaultStorageConstraints(sb, args.Storage, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	if err := validateStorageConstraints(sb.storageBackend, args.Storage, args.Charm.Meta()); err != nil {
		return nil, errors.Trace(err)
	}
	storagePools := make(set.Strings)
	for _, storageParams := range args.Storage {
		storagePools.Add(storageParams.Pool)
	}

	// Always ensure that we snapshot the application architecture when adding
	// the application. If no architecture in the constraints, then look at
	// the model constraints. If no architecture is found in the model, use the
	// default architecture (amd64).
	var (
		cons        = args.Constraints
		subordinate = args.Charm.Meta().Subordinate
	)
	if !subordinate && !cons.HasArch() {
		// TODO(CodingCookieRookie): Retrieve model constraints to be used as second arg in ArchOrDefault below
		a := constraints.ArchOrDefault(cons, nil)
		cons.Arch = &a
		args.Constraints = cons
	}

	// Perform model specific arg processing.
	var (
		placement    string
		hasResources bool
	)
	switch model.Type() {
	case ModelTypeIAAS:
		if err := st.processIAASModelApplicationArgs(&args); err != nil {
			return nil, errors.Trace(err)
		}
	case ModelTypeCAAS:
		hasResources = true // all k8s apps start with the assumption of resources
		if err := st.processCAASModelApplicationArgs(&args); err != nil {
			return nil, errors.Trace(err)
		}
		if len(args.Placement) == 1 {
			placement = args.Placement[0].Directive
		}
	}

	applicationID := st.docID(args.Name)

	// The doc defaults to CharmModifiedVersion = 0, which is correct, since it
	// has, by definition, at its initial state.
	cURL := args.CharmURL
	appDoc := &applicationDoc{
		DocID:       applicationID,
		Name:        args.Name,
		ModelUUID:   st.ModelUUID(),
		Subordinate: subordinate,
		CharmURL:    &cURL,
		CharmOrigin: *args.CharmOrigin,
		Life:        Alive,
		UnitCount:   args.NumUnits,

		// CAAS
		Placement:    placement,
		HasResources: hasResources,
	}

	app := newApplication(st, appDoc)

	if err := args.ApplicationConfig.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	appConfigAttrs := args.ApplicationConfig.Attributes()

	// When creating the settings, we ignore nils.  In other circumstances, nil
	// means to delete the value (reset to default), so creating with nil should
	// mean to use the default, i.e. don't set the value.
	removeNils(args.CharmConfig)
	removeNils(appConfigAttrs)

	var unitNames []string
	buildTxn := func(attempt int) ([]txn.Op, error) {
		unitNames = []string{}
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
		}
		// The addApplicationOps does not include the model alive assertion,
		// so we add it here.
		ops := []txn.Op{
			assertModelActiveOp(st.ModelUUID()),
		}
		addOps, err := addApplicationOps(st, app, addApplicationOpsArgs{
			applicationDoc:    appDoc,
			constraints:       args.Constraints,
			storage:           args.Storage,
			applicationConfig: appConfigAttrs,
			charmConfig:       args.CharmConfig,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		ops = append(ops, addOps...)

		if err := resetSequence(st, app.Tag().String()); err != nil {
			return nil, errors.Trace(err)
		}

		// Collect unit-adding operations.
		for x := 0; x < args.NumUnits; x++ {
			unitName, unitOps, err := app.addUnitOpsWithCons(
				applicationAddUnitOpsArgs{
					cons:          args.Constraints,
					storageCons:   args.Storage,
					attachStorage: args.AttachStorage,
					charmMeta:     args.Charm.Meta(),
				},
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			unitNames = append(unitNames, unitName)
			ops = append(ops, unitOps...)
			if model.Type() != ModelTypeCAAS {
				placement := instance.Placement{}
				if x < len(args.Placement) {
					placement = *args.Placement[x]
				}
				ops = append(ops, assignUnitOps(unitName, placement)...)
			}
		}
		return ops, nil
	}

	if err = st.db().Run(buildTxn); err == nil {
		// Refresh to pick the txn-revno.
		if err = app.refresh(); err != nil {
			return nil, errors.Trace(err)
		}
		return app, nil
	}
	return nil, errors.Trace(err)
}

func (st *State) processIAASModelApplicationArgs(args *AddApplicationArgs) error {
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
		if errors.Is(err, errors.NotFound) {
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

			// This placement directive indicates that we're putting a
			// unit on a pre-existing machine. There's no need to
			// precheck the args since we're not starting an instance.
		}
	}

	return nil
}

func (st *State) processCAASModelApplicationArgs(args *AddApplicationArgs) error {
	if len(args.Placement) > 0 {
		return errors.NotValidf("placement directives on k8s models")
	}
	return nil
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

// Application returns an application state by name.
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

// Report conforms to the Dependency Engine Report() interface, giving an opportunity to introspect
// what is going on at runtime.
func (st *State) Report() map[string]interface{} {
	if st.workers == nil {
		return nil
	}
	return st.workers.Report()
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

// TagFromDocID tries attempts to extract an entity-identifying tag from a
// Mongo document ID.
// For example "c9741ea1-0c2a-444d-82f5-787583a48557:a#mediawiki" would yield
// an application tag for "mediawiki"
func TagFromDocID(docID string) names.Tag {
	_, localID, _ := splitDocID(docID)
	switch {
	case strings.HasPrefix(localID, "a#"):
		return names.NewApplicationTag(localID[2:])
	case strings.HasPrefix(localID, "m#"):
		return names.NewMachineTag(localID[2:])
	case strings.HasPrefix(localID, "u#"):
		return names.NewUnitTag(localID[2:])
	case strings.HasPrefix(localID, "e"):
		return names.NewModelTag(docID)
	default:
		return nil
	}
}
