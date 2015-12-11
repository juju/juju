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

// Environment represents the state of an environment.
type Environment struct {
	// st is not neccessarly the state of this environment. Though it is
	// usually safe to assume that it is. The only times it isn't is when we
	// get environments other than the current one - which is mostly in
	// controller api endpoints.
	st  *State
	doc environmentDoc
}

// environmentDoc represents the internal state of the environment in MongoDB.
type environmentDoc struct {
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
func (st *State) ControllerEnvironment() (*Environment, error) {
	ssinfo, err := st.StateServerInfo()
	if err != nil {
		return nil, errors.Annotate(err, "could not get state server info")
	}

	environments, closer := st.getCollection(environmentsC)
	defer closer()

	env := &Environment{st: st}
	uuid := ssinfo.EnvironmentTag.Id()
	if err := env.refresh(environments.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	env := &Environment{st: st}
	uuid := st.environTag.Id()
	if err := env.refresh(environments.FindId(uuid)); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// GetEnvironment looks for the environment identified by the uuid passed in.
func (st *State) GetEnvironment(tag names.EnvironTag) (*Environment, error) {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	env := &Environment{st: st}
	if err := env.refresh(environments.FindId(tag.Id())); err != nil {
		return nil, errors.Trace(err)
	}
	return env, nil
}

// AllEnvironments returns all the environments in the system.
func (st *State) AllEnvironments() ([]*Environment, error) {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	var envDocs []environmentDoc
	err := environments.Find(nil).All(&envDocs)
	if err != nil {
		return nil, err
	}

	result := make([]*Environment, len(envDocs))
	for i, doc := range envDocs {
		result[i] = &Environment{st: st, doc: doc}
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
func (st *State) NewEnvironment(cfg *config.Config, owner names.UserTag) (_ *Environment, _ *State, err error) {
	if owner.IsLocal() {
		if _, err := st.User(owner); err != nil {
			return nil, nil, errors.Annotate(err, "cannot create environment")
		}
	}

	ssEnv, err := st.ControllerEnvironment()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not load state server environment")
	}

	uuid, ok := cfg.UUID()
	if !ok {
		return nil, nil, errors.Errorf("environment uuid was not supplied")
	}
	newState, err := st.ForEnviron(names.NewEnvironTag(uuid))
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not create state for new environment")
	}
	defer func() {
		if err != nil {
			newState.Close()
		}
	}()

	ops, err := newState.envSetupOps(cfg, uuid, ssEnv.UUID(), owner)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new environment")
	}
	err = newState.runTransaction(ops)
	if err == txn.ErrAborted {

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		environments, closer := st.getCollection(environmentsC)
		defer closer()
		envCount, countErr := environments.Find(bson.D{
			{"owner", owner.Canonical()},
			{"name", cfg.Name()}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if envCount > 0 {
			err = errors.AlreadyExistsf("environment %q for %s", cfg.Name(), owner.Canonical())
		} else {
			err = errors.New("environment already exists")
		}
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	newEnv, err := newState.Environment()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	return newEnv, newState, nil
}

// Tag returns a name identifying the environment.
// The returned name will be different from other Tag values returned
// by any other entities from the same state.
func (e *Environment) Tag() names.Tag {
	return e.EnvironTag()
}

// EnvironTag is the concrete environ tag for this environment.
func (e *Environment) EnvironTag() names.EnvironTag {
	return names.NewEnvironTag(e.doc.UUID)
}

// ControllerTag is the environ tag for the controller that the environment is
// running within.
func (e *Environment) ControllerTag() names.EnvironTag {
	return names.NewEnvironTag(e.doc.ServerUUID)
}

// UUID returns the universally unique identifier of the environment.
func (e *Environment) UUID() string {
	return e.doc.UUID
}

// ControllerUUID returns the universally unique identifier of the controller
// in which the environment is running.
func (e *Environment) ControllerUUID() string {
	return e.doc.ServerUUID
}

// Name returns the human friendly name of the environment.
func (e *Environment) Name() string {
	return e.doc.Name
}

// Life returns whether the environment is Alive, Dying or Dead.
func (e *Environment) Life() Life {
	return e.doc.Life
}

// TimeOfDying returns when the environment Life was set to Dying.
func (e *Environment) TimeOfDying() time.Time {
	return e.doc.TimeOfDying
}

// TimeOfDeath returns when the environment Life was set to Dead.
func (e *Environment) TimeOfDeath() time.Time {
	return e.doc.TimeOfDeath
}

// Owner returns tag representing the owner of the environment.
// The owner is the user that created the environment.
func (e *Environment) Owner() names.UserTag {
	return names.NewUserTag(e.doc.Owner)
}

// Config returns the config for the environment.
func (e *Environment) Config() (*config.Config, error) {
	if e.st.environTag.Id() == e.UUID() {
		return e.st.EnvironConfig()
	}
	envState := e.st
	if envState.environTag != e.EnvironTag() {
		// The active environment isn't the same as the environment
		// we are querying.
		var err error
		envState, err = e.st.ForEnviron(e.EnvironTag())
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer envState.Close()
	}
	return envState.EnvironConfig()
}

// UpdateLatestToolsVersion looks up for the latest available version of
// juju tools and updates environementDoc with it.
func (e *Environment) UpdateLatestToolsVersion(ver version.Number) error {
	v := ver.String()
	// TODO(perrito666): I need to assert here that there isn't a newer
	// version in place.
	ops := []txn.Op{{
		C:      environmentsC,
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
func (e *Environment) LatestToolsVersion() version.Number {
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
func (e *Environment) globalKey() string {
	return environGlobalKey
}

func (e *Environment) Refresh() error {
	environments, closer := e.st.getCollection(environmentsC)
	defer closer()
	return e.refresh(environments.FindId(e.UUID()))
}

func (e *Environment) refresh(query *mgo.Query) error {
	err := query.One(&e.doc)
	if err == mgo.ErrNotFound {
		return errors.NotFoundf("environment")
	}
	return err
}

// Users returns a slice of all users for this environment.
func (e *Environment) Users() ([]*EnvironmentUser, error) {
	if e.st.EnvironUUID() != e.UUID() {
		return nil, errors.New("cannot lookup environment users outside the current environment")
	}
	coll, closer := e.st.getCollection(envUsersC)
	defer closer()

	var userDocs []envUserDoc
	err := coll.Find(nil).All(&userDocs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var envUsers []*EnvironmentUser
	for _, doc := range userDocs {
		envUsers = append(envUsers, &EnvironmentUser{
			st:  e.st,
			doc: doc,
		})
	}

	return envUsers, nil
}

// Destroy sets the environment's lifecycle to Dying, preventing
// addition of services or machines to state.
func (e *Environment) Destroy() (err error) {
	return e.destroy(false)
}

// Destroy sets the environment's lifecycle to Dying, preventing addition of
// services or machines to state. If this environment is a controller hosting
// other environments, they will also be destroyed.
func (e *Environment) DestroyIncludingHosted() error {
	return e.destroy(true)
}

func (e *Environment) destroy(destroyHostedEnvirons bool) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to destroy environment")

	buildTxn := func(attempt int) ([]txn.Op, error) {

		// On the first attempt, we assume memory state is recent
		// enough to try using...
		if attempt != 0 {
			// ...but on subsequent attempts, we read fresh environ
			// state from the DB. Note that we do *not* refresh `e`
			// itself, as detailed in doc/hacking-state.txt
			if e, err = e.st.GetEnvironment(e.EnvironTag()); err != nil {
				return nil, errors.Trace(err)
			}
		}

		ops, err := e.destroyOps(destroyHostedEnvirons)
		if err == errEnvironNotAlive {
			return nil, jujutxn.ErrNoOperations
		} else if err != nil {
			return nil, errors.Trace(err)
		}

		return ops, nil
	}

	st := e.st
	if e.UUID() != e.st.EnvironUUID() {
		st, err = e.st.ForEnviron(e.EnvironTag())
		defer st.Close()
		if err != nil {
			return errors.Trace(err)
		}
	}
	return st.run(buildTxn)
}

// errEnvironNotAlive is a signal emitted from destroyOps to indicate
// that environment destruction is already underway.
var errEnvironNotAlive = errors.New("environment is no longer alive")

type hasHostedEnvironsError int

func (e hasHostedEnvironsError) Error() string {
	return fmt.Sprintf("hosting %d other environments", e)
}

func IsHasHostedEnvironsError(err error) bool {
	_, ok := errors.Cause(err).(hasHostedEnvironsError)
	return ok
}

// destroyOps returns the txn operations necessary to begin environ
// destruction, or an error indicating why it can't.
func (e *Environment) destroyOps(destroyHostedEnvirons bool) ([]txn.Op, error) {
	// Ensure we're using the environment's state.
	st := e.st
	var err error
	if e.UUID() != e.st.EnvironUUID() {
		st, err = e.st.ForEnviron(e.EnvironTag())
		defer st.Close()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if e.Life() != Alive {
		return nil, errEnvironNotAlive
	}

	err = e.ensureDestroyable()
	if err != nil {
		return nil, errors.Trace(err)
	}

	uuid := e.UUID()
	ops := []txn.Op{{
		C:      environmentsC,
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
func (e *Environment) ensureDestroyable() error {

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
	doc := &environmentDoc{
		UUID:       uuid,
		Name:       name,
		Life:       Alive,
		Owner:      owner.Canonical(),
		ServerUUID: server,
	}
	return txn.Op{
		C:      environmentsC,
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
// an userenvnameC document with the given owner and environment name.
func createUniqueOwnerEnvNameOp(owner names.UserTag, envName string) txn.Op {
	return txn.Op{
		C:      userenvnameC,
		Id:     userEnvNameIndex(owner.Canonical(), envName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertAliveOp returns a txn.Op that asserts the environment is alive.
func (e *Environment) assertAliveOp() txn.Op {
	return assertEnvAliveOp(e.UUID())
}

// assertEnvAliveOp returns a txn.Op that asserts the given
// environment UUID refers to an Alive environment.
func assertEnvAliveOp(envUUID string) txn.Op {
	return txn.Op{
		C:      environmentsC,
		Id:     envUUID,
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
	env, err := st.Environment()
	if (err == nil && env.Life() != Alive) || errors.IsNotFound(err) {
		return errors.New("environment is no longer alive")
	} else if err != nil {
		return errors.Annotate(err, "unable to read environment")
	}
	return nil
}
