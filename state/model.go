// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jujutxn "github.com/juju/txn"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/version"
)

// modelGlobalKey is the key for the model, its
// settings and constraints.
const modelGlobalKey = "e"

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
	UUID        string `bson:"_id"`
	Name        string
	Life        Life
	Owner       string    `bson:"owner"`
	ServerUUID  string    `bson:"server-uuid"`
	TimeOfDying time.Time `bson:"time-of-dying"`
	TimeOfDeath time.Time `bson:"time-of-death"`

	// LatestAvailableTools is a string representing the newest version
	// found while checking streams for new versions.
	LatestAvailableTools string `bson:"available-tools,omitempty"`
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

	env := &Model{st: st}
	uuid := st.modelTag.Id()
	if err := env.refresh(models.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
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

// NewModel creates a new model with its own UUID and
// prepares it for use. Model and State instances for the new
// model are returned.
//
// The controller model's UUID is attached to the new
// model's document. Having the server UUIDs stored with each
// model document means that we have a way to represent external
// models, perhaps for future use around cross model
// relations.
func (st *State) NewModel(cfg *config.Config, owner names.UserTag) (_ *Model, _ *State, err error) {
	if owner.IsLocal() {
		if _, err := st.User(owner); err != nil {
			return nil, nil, errors.Annotate(err, "cannot create model")
		}
	}

	ssEnv, err := st.ControllerModel()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not load controller model")
	}

	uuid, ok := cfg.UUID()
	if !ok {
		return nil, nil, errors.Errorf("model uuid was not supplied")
	}
	newState, err := st.ForModel(names.NewModelTag(uuid))
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not create state for new model")
	}
	defer func() {
		if err != nil {
			newState.Close()
		}
	}()

	ops, err := newState.envSetupOps(cfg, uuid, ssEnv.UUID(), owner)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new model")
	}
	err = newState.runTransaction(ops)
	if err == txn.ErrAborted {

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		models, closer := st.getCollection(modelsC)
		defer closer()
		envCount, countErr := models.Find(bson.D{
			{"owner", owner.Canonical()},
			{"name", cfg.Name()}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if envCount > 0 {
			err = errors.AlreadyExistsf("model %q for %s", cfg.Name(), owner.Canonical())
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

// Life returns whether the model is Alive, Dying or Dead.
func (m *Model) Life() Life {
	return m.doc.Life
}

// TimeOfDying returns when the model Life was set to Dying.
func (m *Model) TimeOfDying() time.Time {
	return m.doc.TimeOfDying
}

// TimeOfDeath returns when the model Life was set to Dead.
func (m *Model) TimeOfDeath() time.Time {
	return m.doc.TimeOfDeath
}

// Owner returns tag representing the owner of the model.
// The owner is the user that created the model.
func (m *Model) Owner() names.UserTag {
	return names.NewUserTag(m.doc.Owner)
}

// Config returns the config for the model.
func (m *Model) Config() (*config.Config, error) {
	if m.st.modelTag.Id() == m.UUID() {
		return m.st.ModelConfig()
	}
	envState := m.st
	if envState.modelTag != m.ModelTag() {
		// The active model isn't the same as the model
		// we are querying.
		var err error
		envState, err = m.st.ForModel(m.ModelTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer envState.Close()
	}
	return envState.ModelConfig()
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

func (m *Model) refresh(query *mgo.Query) error {
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
// addition of services or machines to state.
func (m *Model) Destroy() (err error) {
	return m.destroy(false)
}

// DestroyIncludingHosted sets the model's lifecycle to Dying, preventing addition of
// services or machines to state. If this model is a controller hosting
// other model, they will also be destroyed.
func (m *Model) DestroyIncludingHosted() error {
	return m.destroy(true)
}

func (m *Model) destroy(destroyHostedModels bool) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to destroy model")

	buildTxn := func(attempt int) ([]txn.Op, error) {

		// On the first attempt, we assume memory state is recent
		// enough to try using...
		if attempt != 0 {
			// ...but on subsequent attempts, we read fresh environ
			// state from the DB. Note that we do *not* refresh `e`
			// itself, as detailed in doc/hacking-state.txt
			if m, err = m.st.GetModel(m.ModelTag()); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops, err := m.destroyOps(destroyHostedModels)
		if err == errModelNotAlive {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		return ops, nil
	}

	st := m.st
	if m.UUID() != m.st.ModelUUID() {
		st, err = m.st.ForModel(m.ModelTag())
		defer st.Close()
		if err != nil {
			return errors.Trace(err)
		}
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
func (m *Model) destroyOps(destroyHostedModels bool) ([]txn.Op, error) {
	// Ensure we're using the model's state.
	st := m.st
	var err error
	if m.UUID() != m.st.ModelUUID() {
		st, err = m.st.ForModel(m.ModelTag())
		defer st.Close()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if m.Life() != Alive {
		return nil, errModelNotAlive
	}

	err = m.ensureDestroyable()
	if err != nil {
		return nil, errors.Trace(err)
	}

	uuid := m.UUID()
	ops := []txn.Op{{
		C:      modelsC,
		Id:     uuid,
		Assert: isModelAliveDoc,
		Update: bson.D{{"$set", bson.D{
			{"life", Dying},
			{"time-of-dying", nowToTheSecond()},
		}}},
	}}
	if uuid == m.doc.ServerUUID && !destroyHostedModels {
		if count, err := hostedModelCount(st); err != nil {
			return nil, errors.Trace(err)
		} else if count != 0 {
			return nil, errors.Trace(hasHostedModelsError(count))
		}
		ops = append(ops, assertNoHostedModelsOp())
	} else {
		// If this model isn't hosting any models, no further
		// checks are necessary -- we just need to make sure we update the
		// refcount.
		ops = append(ops, decHostedModelCountOp())
	}

	// Because txn operations execute in order, and may encounter
	// arbitrarily long delays, we need to make sure every op
	// causes a state change that's still consistent; so we make
	// sure the cleanup ops are the last thing that will execute.
	if uuid == m.doc.ServerUUID {
		cleanupOp := st.newCleanupOp(cleanupModelsForDyingController, uuid)
		ops = append(ops, cleanupOp)
	}
	cleanupMachinesOp := st.newCleanupOp(cleanupMachinesForDyingModel, uuid)
	ops = append(ops, cleanupMachinesOp)
	cleanupServicesOp := st.newCleanupOp(cleanupServicesForDyingModel, uuid)
	ops = append(ops, cleanupServicesOp)
	return ops, nil
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
func (m *Model) ensureDestroyable() error {

	// TODO(waigani) bug #1475212: Model destroy can miss manual
	// machines. We need to be able to assert the absence of these as
	// part of the destroy txn, but in order to do this  manual machines
	// need to add refcounts to their models.

	// Check for manual machines. We bail out if there are any,
	// to stop the user from prematurely hobbling the model.
	machines, err := m.st.AllMachines()
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
func createModelOp(st *State, owner names.UserTag, name, uuid, server string) txn.Op {
	doc := &modelDoc{
		UUID:       uuid,
		Name:       name,
		Life:       Alive,
		Owner:      owner.Canonical(),
		ServerUUID: server,
	}
	return txn.Op{
		C:      modelsC,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

const hostedModelCountKey = "hostedModelCount"

type hostedModelCountDoc struct {

	// RefCount is the number of models in the Juju system. We do not count
	// the system model.
	RefCount int `bson:"refcount"`
}

func assertNoHostedModelsOp() txn.Op {
	return txn.Op{
		C:      controllersC,
		Id:     hostedModelCountKey,
		Assert: bson.D{{"refcount", 0}},
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
		C:  controllersC,
		Id: hostedModelCountKey,
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
func (m *Model) assertAliveOp() txn.Op {
	return assertModelAliveOp(m.UUID())
}

// assertModelAliveOp returns a txn.Op that asserts the given
// model UUID refers to an Alive model.
func assertModelAliveOp(modelUUID string) txn.Op {
	return txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: isModelAliveDoc,
	}
}

// isModelAlive is a model-specific version of isAliveDoc.
//
// Model documents from versions of Juju prior to 1.17
// do not have the life field; if it does not exist, it should
// be considered to have the value Alive.
//
// TODO(mjs) - this should be removed with existing uses replaced with
// isAliveDoc. A DB migration should convert nil to Alive.
var isModelAliveDoc = bson.D{
	{"life", bson.D{{"$in", []interface{}{Alive, nil}}}},
}

func checkModeLife(st *State) error {
	env, err := st.Model()
	if (err == nil && env.Life() != Alive) || errors.IsNotFound(err) {
		return errors.New("model is no longer alive")
	} else if err != nil {
		return errors.Annotate(err, "unable to read model")
	}
	return nil
}
