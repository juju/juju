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

// environGlobalKey is the key for the environment, its
// settings and constraints.
const environGlobalKey = "e"

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

// ControllerEnvironment returns the environment that was bootstrapped.
// This is the only environment that can have state server machines.
// The owner of this environment is also considered "special", in that
// they are the only user that is able to create other users (until we
// have more fine grained permissions), and they cannot be disabled.
func (st *State) ControllerEnvironment() (*Model, error) {
	ssinfo, err := st.StateServerInfo()
	if err != nil {
		return nil, errors.Annotate(err, "could not get state server info")
	}

	environments, closer := st.getCollection(modelsC)
	defer closer()

	env := &Model{st: st}
	uuid := ssinfo.ModelTag.Id()
	if err := env.refresh(environments.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// Environment returns the environment entity.
func (st *State) Model() (*Model, error) {
	environments, closer := st.getCollection(modelsC)
	defer closer()

	env := &Model{st: st}
	uuid := st.modelTag.Id()
	if err := env.refresh(environments.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// GetEnvironment looks for the environment identified by the uuid passed in.
func (st *State) GetEnvironment(tag names.ModelTag) (*Model, error) {
	environments, closer := st.getCollection(modelsC)
	defer closer()

	env := &Model{st: st}
	if err := env.refresh(environments.FindId(tag.Id())); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// AllEnvironments returns all the environments in the system.
func (st *State) AllEnvironments() ([]*Model, error) {
	environments, closer := st.getCollection(modelsC)
	defer closer()

	var envDocs []modelDoc
	err := environments.Find(nil).All(&envDocs)
	if err != nil {
		return nil, err
	}

	result := make([]*Model, len(envDocs))
	for i, doc := range envDocs {
		result[i] = &Model{st: st, doc: doc}
	}

	return result, nil
}

// NewEnvironment creates a new environment with its own UUID and
// prepares it for use. Environment and State instances for the new
// environment are returned.
//
// The state server environment's UUID is attached to the new
// environment's document. Having the server UUIDs stored with each
// environment document means that we have a way to represent external
// environments, perhaps for future use around cross environment
// relations.
func (st *State) NewEnvironment(cfg *config.Config, owner names.UserTag) (_ *Model, _ *State, err error) {
	if owner.IsLocal() {
		if _, err := st.User(owner); err != nil {
			return nil, nil, errors.Annotate(err, "cannot create model")
		}
	}

	ssEnv, err := st.ControllerEnvironment()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not load controller model")
	}

	uuid, ok := cfg.UUID()
	if !ok {
		return nil, nil, errors.Errorf("model uuid was not supplied")
	}
	newState, err := st.ForEnviron(names.NewModelTag(uuid))
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
		environments, closer := st.getCollection(modelsC)
		defer closer()
		envCount, countErr := environments.Find(bson.D{
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

// Tag returns a name identifying the environment.
// The returned name will be different from other Tag values returned
// by any other entities from the same state.
func (e *Model) Tag() names.Tag {
	return e.ModelTag()
}

// ModelTag is the concrete environ tag for this environment.
func (e *Model) ModelTag() names.ModelTag {
	return names.NewModelTag(e.doc.UUID)
}

// ControllerTag is the environ tag for the controller that the environment is
// running within.
func (e *Model) ControllerTag() names.ModelTag {
	return names.NewModelTag(e.doc.ServerUUID)
}

// UUID returns the universally unique identifier of the environment.
func (e *Model) UUID() string {
	return e.doc.UUID
}

// ControllerUUID returns the universally unique identifier of the controller
// in which the environment is running.
func (e *Model) ControllerUUID() string {
	return e.doc.ServerUUID
}

// Name returns the human friendly name of the environment.
func (e *Model) Name() string {
	return e.doc.Name
}

// Life returns whether the environment is Alive, Dying or Dead.
func (e *Model) Life() Life {
	return e.doc.Life
}

// TimeOfDying returns when the environment Life was set to Dying.
func (e *Model) TimeOfDying() time.Time {
	return e.doc.TimeOfDying
}

// TimeOfDeath returns when the environment Life was set to Dead.
func (e *Model) TimeOfDeath() time.Time {
	return e.doc.TimeOfDeath
}

// Owner returns tag representing the owner of the environment.
// The owner is the user that created the environment.
func (e *Model) Owner() names.UserTag {
	return names.NewUserTag(e.doc.Owner)
}

// Config returns the config for the environment.
func (e *Model) Config() (*config.Config, error) {
	if e.st.modelTag.Id() == e.UUID() {
		return e.st.EnvironConfig()
	}
	envState := e.st
	if envState.modelTag != e.ModelTag() {
		// The active environment isn't the same as the environment
		// we are querying.
		var err error
		envState, err = e.st.ForEnviron(e.ModelTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer envState.Close()
	}
	return envState.EnvironConfig()
}

// UpdateLatestToolsVersion looks up for the latest available version of
// juju tools and updates environementDoc with it.
func (e *Model) UpdateLatestToolsVersion(ver version.Number) error {
	v := ver.String()
	// TODO(perrito666): I need to assert here that there isn't a newer
	// version in place.
	ops := []txn.Op{{
		C:      modelsC,
		Id:     e.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"available-tools", v}}}},
	}}
	err := e.st.runTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}
	return e.Refresh()
}

// LatestToolsVersion returns the newest version found in the last
// check in the streams.
// Bear in mind that the check was performed filtering only
// new patches for the current major.minor. (major.minor.patch)
func (e *Model) LatestToolsVersion() version.Number {
	ver := e.doc.LatestAvailableTools
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

// globalKey returns the global database key for the environment.
func (e *Model) globalKey() string {
	return environGlobalKey
}

func (e *Model) Refresh() error {
	environments, closer := e.st.getCollection(modelsC)
	defer closer()
	return e.refresh(environments.FindId(e.UUID()))
}

func (e *Model) refresh(query *mgo.Query) error {
	err := query.One(&e.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("model")
	}
	return err
}

// Users returns a slice of all users for this environment.
func (e *Model) Users() ([]*ModelUser, error) {
	if e.st.ModelUUID() != e.UUID() {
		return nil, errors.New("cannot lookup model users outside the current model")
	}
	coll, closer := e.st.getCollection(modelUsersC)
	defer closer()

	var userDocs []modelUserDoc
	err := coll.Find(nil).All(&userDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var envUsers []*ModelUser
	for _, doc := range userDocs {
		envUsers = append(envUsers, &ModelUser{
			st:  e.st,
			doc: doc,
		})
	}

	return envUsers, nil
}

// Destroy sets the models's lifecycle to Dying, preventing
// addition of services or machines to state.
func (e *Model) Destroy() (err error) {
	return e.destroy(false)
}

// DestroyIncludingHosted sets the model's lifecycle to Dying, preventing addition of
// services or machines to state. If this model is a controller hosting
// other model, they will also be destroyed.
func (e *Model) DestroyIncludingHosted() error {
	return e.destroy(true)
}

func (e *Model) destroy(destroyHostedModels bool) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to destroy model")

	buildTxn := func(attempt int) ([]txn.Op, error) {

		// On the first attempt, we assume memory state is recent
		// enough to try using...
		if attempt != 0 {
			// ...but on subsequent attempts, we read fresh environ
			// state from the DB. Note that we do *not* refresh `e`
			// itself, as detailed in doc/hacking-state.txt
			if e, err = e.st.GetEnvironment(e.ModelTag()); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops, err := e.destroyOps(destroyHostedModels)
		if err == errModelNotAlive {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		return ops, nil
	}

	st := e.st
	if e.UUID() != e.st.ModelUUID() {
		st, err = e.st.ForEnviron(e.ModelTag())
		defer st.Close()
		if err != nil {
			return errors.Trace(err)
		}
	}
	return st.run(buildTxn)
}

// errModelNotAlive is a signal emitted from destroyOps to indicate
// that environment destruction is already underway.
var errModelNotAlive = errors.New("model is no longer alive")

type hasHostedEnvironsError int

func (e hasHostedEnvironsError) Error() string {
	return fmt.Sprintf("hosting %d other models", e)
}

func IsHasHostedEnvironsError(err error) bool {
	_, ok := errors.Cause(err).(hasHostedEnvironsError)
	return ok
}

// destroyOps returns the txn operations necessary to begin environ
// destruction, or an error indicating why it can't.
func (e *Model) destroyOps(destroyHostedEnvirons bool) ([]txn.Op, error) {
	// Ensure we're using the environment's state.
	st := e.st
	var err error
	if e.UUID() != e.st.ModelUUID() {
		st, err = e.st.ForEnviron(e.ModelTag())
		defer st.Close()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if e.Life() != Alive {
		return nil, errModelNotAlive
	}

	err = e.ensureDestroyable()
	if err != nil {
		return nil, errors.Trace(err)
	}

	uuid := e.UUID()
	ops := []txn.Op{{
		C:      modelsC,
		Id:     uuid,
		Assert: isEnvAliveDoc,
		Update: bson.D{{"$set", bson.D{
			{"life", Dying},
			{"time-of-dying", nowToTheSecond()},
		}}},
	}}
	if uuid == e.doc.ServerUUID && !destroyHostedEnvirons {
		if count, err := hostedEnvironCount(st); err != nil {
			return nil, errors.Trace(err)
		} else if count != 0 {
			return nil, errors.Trace(hasHostedEnvironsError(count))
		}
		ops = append(ops, assertNoHostedEnvironsOp())
	} else {
		// If this environment isn't hosting any environments, no further
		// checks are necessary -- we just need to make sure we update the
		// refcount.
		ops = append(ops, decHostedEnvironCountOp())
	}

	// Because txn operations execute in order, and may encounter
	// arbitrarily long delays, we need to make sure every op
	// causes a state change that's still consistent; so we make
	// sure the cleanup ops are the last thing that will execute.
	if uuid == e.doc.ServerUUID {
		cleanupOp := st.newCleanupOp(cleanupEnvironmentsForDyingController, uuid)
		ops = append(ops, cleanupOp)
	}
	cleanupMachinesOp := st.newCleanupOp(cleanupMachinesForDyingEnvironment, uuid)
	ops = append(ops, cleanupMachinesOp)
	cleanupServicesOp := st.newCleanupOp(cleanupServicesForDyingEnvironment, uuid)
	ops = append(ops, cleanupServicesOp)
	return ops, nil
}

// checkManualMachines checks if any of the machines in the slice were
// manually provisioned, and are non-manager machines. These machines
// must (currently) be manually destroyed via destroy-machine before
// destroy-environment can successfully complete.
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
func (e *Model) ensureDestroyable() error {

	// TODO(waigani) bug #1475212: Environment destroy can miss manual
	// machines. We need to be able to assert the absence of these as
	// part of the destroy txn, but in order to do this  manual machines
	// need to add refcounts to their environments.

	// Check for manual machines. We bail out if there are any,
	// to stop the user from prematurely hobbling the environment.
	machines, err := e.st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkManualMachines(machines); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// createEnvironmentOp returns the operation needed to create
// an environment document with the given name and UUID.
func createEnvironmentOp(st *State, owner names.UserTag, name, uuid, server string) txn.Op {
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

const hostedEnvCountKey = "hostedEnvironCount"

type hostedEnvCountDoc struct {

	// RefCount is the number of environments in the Juju system. We do not count
	// the system environment.
	RefCount int `bson:"refcount"`
}

func assertNoHostedEnvironsOp() txn.Op {
	return txn.Op{
		C:      stateServersC,
		Id:     hostedEnvCountKey,
		Assert: bson.D{{"refcount", 0}},
	}
}

func incHostedEnvironCountOp() txn.Op {
	return hostedEnvironCountOp(1)
}

func decHostedEnvironCountOp() txn.Op {
	return hostedEnvironCountOp(-1)
}

func hostedEnvironCountOp(amount int) txn.Op {
	return txn.Op{
		C:  stateServersC,
		Id: hostedEnvCountKey,
		Update: bson.M{
			"$inc": bson.M{"refcount": amount},
		},
	}
}

func hostedEnvironCount(st *State) (int, error) {
	var doc hostedEnvCountDoc
	stateServers, closer := st.getCollection(stateServersC)
	defer closer()

	if err := stateServers.Find(bson.D{{"_id", hostedEnvCountKey}}).One(&doc); err != nil {
		return 0, errors.Trace(err)
	}
	return doc.RefCount, nil
}

// createUniqueOwnerEnvNameOp returns the operation needed to create
// an usermodelnameC document with the given owner and environment name.
func createUniqueOwnerEnvNameOp(owner names.UserTag, envName string) txn.Op {
	return txn.Op{
		C:      usermodelnameC,
		Id:     userModelNameIndex(owner.Canonical(), envName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertAliveOp returns a txn.Op that asserts the environment is alive.
func (e *Model) assertAliveOp() txn.Op {
	return assertEnvAliveOp(e.UUID())
}

// assertEnvAliveOp returns a txn.Op that asserts the given
// environment UUID refers to an Alive environment.
func assertEnvAliveOp(modelUUID string) txn.Op {
	return txn.Op{
		C:      modelsC,
		Id:     modelUUID,
		Assert: isEnvAliveDoc,
	}
}

// isEnvAlive is an Environment-specific version of isAliveDoc.
//
// Environment documents from versions of Juju prior to 1.17
// do not have the life field; if it does not exist, it should
// be considered to have the value Alive.
//
// TODO(mjs) - this should be removed with existing uses replaced with
// isAliveDoc. A DB migration should convert nil to Alive.
var isEnvAliveDoc = bson.D{
	{"life", bson.D{{"$in", []interface{}{Alive, nil}}}},
}

func checkEnvLife(st *State) error {
	env, err := st.Model()
	if (err == nil && env.Life() != Alive) || errors.IsNotFound(err) {
		return errors.New("model is no longer alive")
	} else if err != nil {
		return errors.Annotate(err, "unable to read model")
	}
	return nil
}
