// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/status"
	"github.com/juju/version"
)

// modelGlobalKey is the key for the model, its
// settings and constraints.
const modelGlobalKey = "e"

// MigrationMode specifies where the Model is with respect to migration.
type MigrationMode string

const (
	// MigrationModeActive is the default mode for a model and reflects
	// a model that is active within its controller.
	MigrationModeActive MigrationMode = ""

	// MigrationModeExporting reflects a model that is in the process of being
	// exported from one controller to another.
	MigrationModeExporting MigrationMode = "exporting"

	// MigrationModeImporting reflects a model that is being imported into a
	// controller, but is not yet fully active.
	MigrationModeImporting MigrationMode = "importing"
)

// Model represents the state of a model.
type Model struct {
	// st is not necessarily the state of this model. Though it is
	// usually safe to assume that it is. The only times it isn't is when we
	// get models other than the current one - which is mostly in
	// controller api endpoints.
	st  *State
	doc modelDoc
}

// modelDoc represents the internal state of the model in MongoDB.
type modelDoc struct {
	UUID          string `bson:"_id"`
	Name          string
	Life          Life
	Owner         string        `bson:"owner"`
	ServerUUID    string        `bson:"server-uuid"`
	MigrationMode MigrationMode `bson:"migration-mode"`

	// Cloud is the name of the cloud that the model is managed within.
	Cloud string `bson:"cloud"`

	// LatestAvailableTools is a string representing the newest version
	// found while checking streams for new versions.
	LatestAvailableTools string `bson:"available-tools,omitempty"`
}

// modelEntityRefsDoc records references to the top-level entities
// in the model.
type modelEntityRefsDoc struct {
	UUID string `bson:"_id"`

	// Machines contains the names of the top-level machines in the model.
	Machines []string `bson:"machines"`

	// Services contains the names of the services in the model.
	Services []string `bson:"applications"`
}

// ControllerModel returns the model that was bootstrapped.
// This is the only model that can have controller machines.
// The owner of this model is also considered "special", in that
// they are the only user that is able to create other users (until we
// have more fine grained permissions), and they cannot be disabled.
func (st *State) ControllerModel() (*Model, error) {
	ssinfo, err := st.ControllerInfo()
	if err != nil {
		return nil, errors.Annotate(err, "could not get controller info")
	}

	models, closer := st.getCollection(modelsC)
	defer closer()

	env := &Model{st: st}
	uuid := ssinfo.ModelTag.Id()
	if err := env.refresh(models.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// Model returns the model entity.
func (st *State) Model() (*Model, error) {
	models, closer := st.getCollection(modelsC)
	defer closer()

	model := &Model{st: st}
	uuid := st.modelTag.Id()
	if err := model.refresh(models.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return model, nil
}

// GetModel looks for the model identified by the uuid passed in.
func (st *State) GetModel(tag names.ModelTag) (*Model, error) {
	models, closer := st.getCollection(modelsC)
	defer closer()

	env := &Model{st: st}
	if err := env.refresh(models.FindId(tag.Id())); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// AllModels returns all the models in the system.
func (st *State) AllModels() ([]*Model, error) {
	models, closer := st.getCollection(modelsC)
	defer closer()

	var envDocs []modelDoc
	err := models.Find(nil).All(&envDocs)
	if err != nil {
		return nil, err
	}

	result := make([]*Model, len(envDocs))
	for i, doc := range envDocs {
		result[i] = &Model{st: st, doc: doc}
	}

	return result, nil
}

// ModelArgs is a params struct for creating a new model.
type ModelArgs struct {
	// Cloud is the name of the cloud that the model is deployed to.
	Cloud string

	// Config is the model config.
	Config *config.Config

	// Owner is the user that owns the model.
	Owner names.UserTag

	// MigrationMode is the initial migration mode of the model.
	MigrationMode MigrationMode
}

// Validate validates the ModelArgs.
func (m ModelArgs) Validate() error {
	if m.Cloud == "" {
		return errors.NotValidf("empty Cloud")
	}
	if m.Config == nil {
		return errors.NotValidf("nil Config")
	}
	if m.Owner == (names.UserTag{}) {
		return errors.NotValidf("empty Owner")
	}
	switch m.MigrationMode {
	case MigrationModeActive, MigrationModeImporting:
	default:
		return errors.NotValidf("initial migration mode %q", m.MigrationMode)
	}
	return nil
}

// NewModel creates a new model with its own UUID and
// prepares it for use. Model and State instances for the new
// model are returned.
//
// The controller model's UUID is attached to the new
// model's document. Having the server UUIDs stored with each
// model document means that we have a way to represent external
// models, perhaps for future use around cross model
// relations.
func (st *State) NewModel(args ModelArgs) (_ *Model, _ *State, err error) {
	if err := args.Validate(); err != nil {
		return nil, nil, errors.Trace(err)
	}

	owner := args.Owner
	if owner.IsLocal() {
		if _, err := st.User(owner); err != nil {
			return nil, nil, errors.Annotate(err, "cannot create model")
		}
	}

	uuid := args.Config.UUID()
	newState, err := st.ForModel(names.NewModelTag(uuid))
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not create state for new model")
	}
	defer func() {
		if err != nil {
			newState.Close()
		}
	}()

	sharedCfg, err := st.SharedCloudConfig()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not red shared config for new model")
	}
	ops, err := newState.modelSetupOps(args.Config, args.Cloud, sharedCfg, owner, args.MigrationMode)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new model")
	}
	err = newState.runTransaction(ops)
	if err == txn.ErrAborted {

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		name := args.Config.Name()
		models, closer := st.getCollection(modelsC)
		defer closer()
		envCount, countErr := models.Find(bson.D{
			{"owner", owner.Canonical()},
			{"name", name}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if envCount > 0 {
			err = errors.AlreadyExistsf("model %q for %s", name, owner.Canonical())
		} else {
			err = errors.New("model already exists")
		}
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	newEnv, err := newState.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return newEnv, newState, nil
}

// Tag returns a name identifying the model.
// The returned name will be different from other Tag values returned
// by any other entities from the same state.
func (m *Model) Tag() names.Tag {
	return m.ModelTag()
}

// ModelTag is the concrete model tag for this model.
func (m *Model) ModelTag() names.ModelTag {
	return names.NewModelTag(m.doc.UUID)
}

// ControllerTag is the model tag for the controller that the model is
// running within.
func (m *Model) ControllerTag() names.ModelTag {
	return names.NewModelTag(m.doc.ServerUUID)
}

// UUID returns the universally unique identifier of the model.
func (m *Model) UUID() string {
	return m.doc.UUID
}

// ControllerUUID returns the universally unique identifier of the controller
// in which the model is running.
func (m *Model) ControllerUUID() string {
	return m.doc.ServerUUID
}

// Name returns the human friendly name of the model.
func (m *Model) Name() string {
	return m.doc.Name
}

// Cloud returns the name of the cloud that the model is deployed to.
func (m *Model) Cloud() string {
	return m.doc.Cloud
}

// MigrationMode returns whether the model is active or being migrated.
func (m *Model) MigrationMode() MigrationMode {
	return m.doc.MigrationMode
}

// SetMigrationMode updates the migration mode of the model.
func (m *Model) SetMigrationMode(mode MigrationMode) error {
	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()

	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Assert: txn.DocExists,
		Update: bson.D{{"$set", bson.D{{"migration-mode", mode}}}},
	}}
	if err := st.runTransaction(ops); err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
}

// Life returns whether the model is Alive, Dying or Dead.
func (m *Model) Life() Life {
	return m.doc.Life
}

// Owner returns tag representing the owner of the model.
// The owner is the user that created the model.
func (m *Model) Owner() names.UserTag {
	return names.NewUserTag(m.doc.Owner)
}

// Status returns the status of the model.
func (m *Model) Status() (status.StatusInfo, error) {
	st, closeState, err := m.getState()
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}
	defer closeState()
	status, err := getStatus(st, m.globalKey(), "model")
	if err != nil {
		return status, err
	}
	return status, nil
}

// SetStatus sets the status of the model.
func (m *Model) SetStatus(sInfo status.StatusInfo) error {
	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()
	if !status.ValidModelStatus(sInfo.Status) {
		return errors.Errorf("cannot set invalid status %q", sInfo.Status)
	}
	return setStatus(st, setStatusParams{
		badge:     "model",
		globalKey: m.globalKey(),
		status:    sInfo.Status,
		message:   sInfo.Message,
		rawData:   sInfo.Data,
		updated:   sInfo.Since,
	})
}

// Config returns the config for the model.
func (m *Model) Config() (*config.Config, error) {
	st, closeState, err := m.getState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closeState()
	return st.ModelConfig()
}

// UpdateLatestToolsVersion looks up for the latest available version of
// juju tools and updates environementDoc with it.
func (m *Model) UpdateLatestToolsVersion(ver version.Number) error {
	v := ver.String()
	// TODO(perrito666): I need to assert here that there isn't a newer
	// version in place.
	ops := []txn.Op{{
		C:      modelsC,
		Id:     m.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"available-tools", v}}}},
	}}
	err := m.st.runTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return m.Refresh()
}

// LatestToolsVersion returns the newest version found in the last
// check in the streams.
// Bear in mind that the check was performed filtering only
// new patches for the current major.minor. (major.minor.patch)
func (m *Model) LatestToolsVersion() version.Number {
	ver := m.doc.LatestAvailableTools
	if ver == "" {
		return version.Zero
	}
	v, err := version.Parse(ver)
	if err != nil {
		// This is being stored from a valid version but
		// in case this data would beacame corrupt It is not
		// worth to fail because of it.
		return version.Zero
	}
	return v
}

// globalKey returns the global database key for the model.
func (m *Model) globalKey() string {
	return modelGlobalKey
}

func (m *Model) Refresh() error {
	models, closer := m.st.getCollection(modelsC)
	defer closer()
	return m.refresh(models.FindId(m.UUID()))
}

func (m *Model) refresh(query mongo.Query) error {
	err := query.One(&m.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("model")
	}
	return err
}

// Users returns a slice of all users for this model.
func (m *Model) Users() ([]*ModelUser, error) {
	if m.st.ModelUUID() != m.UUID() {
		return nil, errors.New("cannot lookup model users outside the current model")
	}
	coll, closer := m.st.getCollection(modelUsersC)
	defer closer()

	var userDocs []modelUserDoc
	err := coll.Find(nil).All(&userDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var modelUsers []*ModelUser
	for _, doc := range userDocs {
		modelUsers = append(modelUsers, &ModelUser{
			st:  m.st,
			doc: doc,
		})
	}

	return modelUsers, nil
}

// Destroy sets the models's lifecycle to Dying, preventing
// addition of services or machines to state. If called on
// an empty hosted model, the lifecycle will be advanced
// straight to Dead.
//
// If called on a controller model, and that controller is
// hosting any non-Dead models, this method will return an
// error satisfying IsHasHostedsError.
func (m *Model) Destroy() (err error) {
	ensureNoHostedModels := false
	if m.doc.UUID == m.doc.ServerUUID {
		ensureNoHostedModels = true
	}
	return m.destroy(ensureNoHostedModels)
}

// DestroyIncludingHosted sets the model's lifecycle to Dying, preventing
// addition of services or machines to state. If this model is a controller
// hosting other models, they will also be destroyed.
func (m *Model) DestroyIncludingHosted() error {
	ensureNoHostedModels := false
	return m.destroy(ensureNoHostedModels)
}

func (m *Model) destroy(ensureNoHostedModels bool) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to destroy model")

	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()

	buildTxn := func(attempt int) ([]txn.Op, error) {
		// On the first attempt, we assume memory state is recent
		// enough to try using...
		if attempt != 0 {
			// ...but on subsequent attempts, we read fresh environ
			// state from the DB. Note that we do *not* refresh `e`
			// itself, as detailed in doc/hacking-state.txt
			if m, err = st.Model(); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops, err := m.destroyOps(ensureNoHostedModels, false)
		if err == errModelNotAlive {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		return ops, nil
	}

	return st.run(buildTxn)
}

// errModelNotAlive is a signal emitted from destroyOps to indicate
// that model destruction is already underway.
var errModelNotAlive = errors.New("model is no longer alive")

type hasHostedModelsError int

func (e hasHostedModelsError) Error() string {
	return fmt.Sprintf("hosting %d other models", e)
}

func IsHasHostedModelsError(err error) bool {
	_, ok := errors.Cause(err).(hasHostedModelsError)
	return ok
}

// destroyOps returns the txn operations necessary to begin model
// destruction, or an error indicating why it can't.
//
// If ensureNoHostedModels is true, then destroyOps will
// fail if there are any non-Dead hosted models
func (m *Model) destroyOps(ensureNoHostedModels, ensureEmpty bool) ([]txn.Op, error) {
	if m.Life() != Alive {
		return nil, errModelNotAlive
	}

	// Ensure we're using the model's state.
	st, closeState, err := m.getState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer closeState()

	if err := ensureDestroyable(st); err != nil {
		return nil, errors.Trace(err)
	}

	// Check if the model is empty. If it is, we can advance the model's
	// lifecycle state directly to Dead.
	var prereqOps []txn.Op
	checkEmptyErr := m.checkEmpty()
	isEmpty := checkEmptyErr == nil
	uuid := m.UUID()
	if ensureEmpty && !isEmpty {
		return nil, errors.Trace(checkEmptyErr)
	}
	if isEmpty {
		prereqOps = append(prereqOps, txn.Op{
			C:  modelEntityRefsC,
			Id: uuid,
			Assert: bson.D{
				{"machines", bson.D{{"$size", 0}}},
				{"applications", bson.D{{"$size", 0}}},
			},
		})
	}

	if ensureNoHostedModels {
		// Check for any Dying or alive but non-empty models. If there
		// are any, we return an error indicating that there are hosted
		// models.
		models, err := st.AllModels()
		if err != nil {
			return nil, errors.Trace(err)
		}
		var aliveEmpty, aliveNonEmpty, dying, dead int
		for _, model := range models {
			if model.UUID() == m.UUID() {
				// Ignore the controller model.
				continue
			}
			if model.Life() == Dead {
				// Dead hosted models don't affect
				// whether the controller can be
				// destroyed or not, but they are
				// still counted in the hosted models.
				dead++
				continue
			}
			// See if the model is empty, and if it is,
			// get the ops required to destroy it.
			ops, err := model.destroyOps(false, true)
			switch err {
			case errModelNotAlive:
				dying++
			case nil:
				prereqOps = append(prereqOps, ops...)
				aliveEmpty++
			default:
				aliveNonEmpty++
			}
		}
		if dying > 0 || aliveNonEmpty > 0 {
			// There are Dying, or Alive but non-empty models.
			// We cannot destroy the controller without first
			// destroying the models and waiting for them to
			// become Dead.
			return nil, errors.Trace(
				hasHostedModelsError(dying + aliveNonEmpty + aliveEmpty),
			)
		}
		// Ensure that the number of active models has not changed
		// between the query and when the transaction is applied.
		//
		// Note that we assert that each empty model that we intend
		// move to Dead is still Alive, so we're protected from an
		// ABA style problem where an empty model is concurrently
		// removed, and replaced with a non-empty model.
		prereqOps = append(prereqOps, assertHostedModelsOp(aliveEmpty+dead))
	}

	life := Dying
	if isEmpty && uuid != m.doc.ServerUUID {
		// The model is empty, and is not the admin
		// model, so we can move it straight to Dead.
		life = Dead
	}
	timeOfDying := nowToTheSecond()
	modelUpdateValues := bson.D{
		{"life", life},
		{"time-of-dying", timeOfDying},
	}
	if life == Dead {
		modelUpdateValues = append(modelUpdateValues, bson.DocElem{
			"time-of-death", timeOfDying,
		})
	}

	ops := []txn.Op{{
		C:      modelsC,
		Id:     uuid,
		Assert: isAliveDoc,
		Update: bson.D{{"$set", modelUpdateValues}},
	}}

	// Because txn operations execute in order, and may encounter
	// arbitrarily long delays, we need to make sure every op
	// causes a state change that's still consistent; so we make
	// sure the cleanup ops are the last thing that will execute.
	if uuid == m.doc.ServerUUID {
		cleanupOp := st.newCleanupOp(cleanupModelsForDyingController, uuid)
		ops = append(ops, cleanupOp)
	}
	if !isEmpty {
		// We only need to destroy machines and models if the model is
		// non-empty. It wouldn't normally be harmful to enqueue the
		// cleanups otherwise, except for when we're destroying an empty
		// hosted model in the course of destroying the controller. In
		// that case we'll get errors if we try to enqueue hosted-model
		// cleanups, because the cleanups collection is non-global.
		cleanupMachinesOp := st.newCleanupOp(cleanupMachinesForDyingModel, uuid)
		ops = append(ops, cleanupMachinesOp)
		cleanupServicesOp := st.newCleanupOp(cleanupServicesForDyingModel, uuid)
		ops = append(ops, cleanupServicesOp)
	}
	return append(prereqOps, ops...), nil
}

// checkEmpty checks that the machine is empty of any entities that may
// require external resource cleanup. If the machine is not empty, then
// an error will be returned.
func (m *Model) checkEmpty() error {
	st, closeState, err := m.getState()
	if err != nil {
		return errors.Trace(err)
	}
	defer closeState()

	modelEntityRefs, closer := st.getCollection(modelEntityRefsC)
	defer closer()

	var doc modelEntityRefsDoc
	if err := modelEntityRefs.FindId(m.UUID()).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return errors.NotFoundf("entity references doc for model %s", m.UUID())
		}
		return errors.Annotatef(err, "getting entity references for model %s", m.UUID())
	}
	if n := len(doc.Machines); n > 0 {
		return errors.Errorf("model not empty, found %d machine(s)", n)
	}
	if n := len(doc.Services); n > 0 {
		return errors.Errorf("model not empty, found %d services(s)", n)
	}
	return nil
}

func addModelMachineRefOp(st *State, machineId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{"machines", machineId}}}},
	}
}

func removeModelMachineRefOp(st *State, machineId string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Update: bson.D{{"$pull", bson.D{{"machines", machineId}}}},
	}
}

func addModelServiceRefOp(st *State, applicationname string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Assert: txn.DocExists,
		Update: bson.D{{"$addToSet", bson.D{{"applications", applicationname}}}},
	}
}

func removeModelServiceRefOp(st *State, applicationname string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     st.ModelUUID(),
		Update: bson.D{{"$pull", bson.D{{"applications", applicationname}}}},
	}
}

// checkManualMachines checks if any of the machines in the slice were
// manually provisioned, and are non-manager machines. These machines
// must (currently) be manually destroyed via destroy-machine before
// destroy-model can successfully complete.
func checkManualMachines(machines []*Machine) error {
	var ids []string
	for _, m := range machines {
		if m.IsManager() {
			continue
		}
		manual, err := m.IsManual()
		if err != nil {
			return errors.Trace(err)
		}
		if manual {
			ids = append(ids, m.Id())
		}
	}
	if len(ids) > 0 {
		return errors.Errorf("manually provisioned machines must first be destroyed with `juju destroy-machine %s`", strings.Join(ids, " "))
	}
	return nil
}

// ensureDestroyable an error if any manual machines or persistent volumes are
// found.
func ensureDestroyable(st *State) error {

	// TODO(waigani) bug #1475212: Model destroy can miss manual
	// machines. We need to be able to assert the absence of these as
	// part of the destroy txn, but in order to do this  manual machines
	// need to add refcounts to their models.

	// Check for manual machines. We bail out if there are any,
	// to stop the user from prematurely hobbling the model.
	machines, err := st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkManualMachines(machines); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// createModelOp returns the operation needed to create
// an model document with the given name and UUID.
func createModelOp(st *State, owner names.UserTag, name, uuid, server, cloud string, mode MigrationMode) txn.Op {
	doc := &modelDoc{
		UUID:          uuid,
		Name:          name,
		Life:          Alive,
		Owner:         owner.Canonical(),
		ServerUUID:    server,
		MigrationMode: mode,
		Cloud:         cloud,
	}
	return txn.Op{
		C:      modelsC,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

func createModelEntityRefsOp(st *State, uuid string) txn.Op {
	return txn.Op{
		C:      modelEntityRefsC,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: &modelEntityRefsDoc{UUID: uuid},
	}
}

const hostedModelCountKey = "hostedModelCount"

type hostedModelCountDoc struct {
	// RefCount is the number of models in the Juju system.
	// We do not count the system model.
	RefCount int `bson:"refcount"`
}

func assertNoHostedModelsOp() txn.Op {
	return assertHostedModelsOp(0)
}

func assertHostedModelsOp(n int) txn.Op {
	return txn.Op{
		C:      controllersC,
		Id:     hostedModelCountKey,
		Assert: bson.D{{"refcount", n}},
	}
}

func incHostedModelCountOp() txn.Op {
	return HostedModelCountOp(1)
}

func decHostedModelCountOp() txn.Op {
	return HostedModelCountOp(-1)
}

func HostedModelCountOp(amount int) txn.Op {
	return txn.Op{
		C:      controllersC,
		Id:     hostedModelCountKey,
		Assert: txn.DocExists,
		Update: bson.M{
			"$inc": bson.M{"refcount": amount},
		},
	}
}

func hostedModelCount(st *State) (int, error) {
	var doc hostedModelCountDoc
	controllers, closer := st.getCollection(controllersC)
	defer closer()

	if err := controllers.Find(bson.D{{"_id", hostedModelCountKey}}).One(&doc); err != nil {
		return 0, errors.Trace(err)
	}
	return doc.RefCount, nil
}

// createUniqueOwnerModelNameOp returns the operation needed to create
// an usermodelnameC document with the given owner and model name.
func createUniqueOwnerModelNameOp(owner names.UserTag, envName string) txn.Op {
	return txn.Op{
		C:      usermodelnameC,
		Id:     userModelNameIndex(owner.Canonical(), envName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertAliveOp returns a txn.Op that asserts the model is alive.
func (m *Model) assertActiveOp() txn.Op {
	return assertModelActiveOp(m.UUID())
}

// getState returns the State for the model, and a function to close
// it when done. If m.st is the model-specific state, then it will
// be returned and the close function will be a no-op.
//
// TODO(axw) 2016-04-14 #1570269
// find a way to guarantee that every Model is associated with the
// appropriate State. The current work-around is too easy to get wrong.
func (m *Model) getState() (*State, func(), error) {
	if m.st.modelTag == m.ModelTag() {
		return m.st, func() {}, nil
	}
	st, err := m.st.ForModel(m.ModelTag())
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	uuid := st.ModelUUID()
	return st, func() {
		if err := st.Close(); err != nil {
			logger.Errorf("closing temporary state for model %s", uuid)
		}
	}, nil
}

// assertModelActiveOp returns a txn.Op that asserts the given
// model UUID refers to an Alive model.
func assertModelActiveOp(modelUUID string) txn.Op {
	return txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: append(isAliveDoc, bson.DocElem{"migration-mode", MigrationModeActive}),
	}
}

func checkModelActive(st *State) error {
	model, err := st.Model()
	if (err == nil && model.Life() != Alive) || errors.IsNotFound(err) {
		return errors.Errorf("model %q is no longer alive", model.Name())
	} else if err != nil {
		return errors.Annotate(err, "unable to read model")
	} else if mode := model.MigrationMode(); mode != MigrationModeActive {
		return errors.Errorf("model %q is being migrated", model.Name())
	}
	return nil
}
