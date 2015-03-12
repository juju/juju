// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"

	"github.com/juju/juju/environs/config"
)

// environGlobalKey is the key for the environment, its
// settings and constraints.
const environGlobalKey = "e"

// Environment represents the state of an environment.
type Environment struct {
	st  *State
	doc environmentDoc
}

// environmentDoc represents the internal state of the environment in MongoDB.
type environmentDoc struct {
	UUID       string `bson:"_id"`
	Name       string
	Life       Life
	Owner      string `bson:"owner"`
	ServerUUID string `bson:"server-uuid"`
}

// StateServerEnvironment returns the environment that was bootstrapped.
// This is the only environment that can have state server machines.
// The owner of this environment is also considered "special", in that
// they are the only user that is able to create other users (until we
// have more fine grained permissions), and they cannot be disabled.
func (st *State) StateServerEnvironment() (*Environment, error) {
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

	ssEnv, err := st.StateServerEnvironment()
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
	err = newState.runTransactionNoEnvAliveAssert(ops)
	if err == txn.ErrAborted {

		// We have a  unique key restriction on the "owner" and "name" fields,
		// which will cause the insert to fail if there is another record with
		// the same "owner" and "name" in the collection. If the txn is
		// aborted, check if it is due to the unique key restriction.
		environments, closer := st.getCollection(environmentsC)
		defer closer()
		envCount, countErr := environments.Find(bson.D{
			{"owner", owner.Username()},
			{"name", cfg.Name()}},
		).Count()
		if countErr != nil {
			err = errors.Trace(countErr)
		} else if envCount > 0 {
			err = errors.AlreadyExistsf("environment %q for %s", cfg.Name(), owner.Username())
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

// ServerTag is the environ tag for the server that the environment is running
// within.
func (e *Environment) ServerTag() names.EnvironTag {
	return names.NewEnvironTag(e.doc.ServerUUID)
}

// UUID returns the universally unique identifier of the environment.
func (e *Environment) UUID() string {
	return e.doc.UUID
}

// ServerUUID returns the universally unique identifier of the server in which
// the environment is running.
func (e *Environment) ServerUUID() string {
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
	// The active environment isn't the same as the environment
	// we are querying.
	envState, err := e.st.ForEnviron(e.ServerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer envState.Close()
	return envState.EnvironConfig()
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

// Destroy sets the environment's lifecycle to Dying, preventing
// addition of services or machines to state.
func (e *Environment) Destroy() (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to destroy environment")
	if e.Life() != Alive {
		return nil
	}

	if err := e.ensureDestroyable(); err != nil {
		return errors.Trace(err)
	}

	if err := e.startDestroy(); err != nil {
		if abortErr := e.abortDestroy(); abortErr != nil {
			return errors.Annotate(abortErr, err.Error())
		}
		return errors.Trace(err)
	}

	// Check that no new environments or machines were added between the first
	// check and the Environment.startDestroy().
	if err := e.ensureDestroyable(); err != nil {
		if abortErr := e.abortDestroy(); abortErr != nil {
			return errors.Annotate(abortErr, err.Error())
		}
		return errors.Trace(err)
	}

	if err := e.finishDestroy(); err != nil {
		if abortErr := e.abortDestroy(); abortErr != nil {
			return errors.Annotate(abortErr, err.Error())
		}
		return errors.Trace(err)
	}

	return nil
}

func (e *Environment) startDestroy() error {
	// Set the environment to Dying, to lock out new machines and services.
	// This puts the environment into an unusable state.
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     e.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
		Assert: isEnvAliveDoc,
	}}
	err := e.st.runTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}
	e.doc.Life = Dying
	return nil
}

func (e *Environment) finishDestroy() error {
	// We add a cleanup for services, but not for machines; machines are
	// destroyed via the provider interface. The exception to this rule is
	// manual machines; the API prevents destroy-environment from succeeding
	// if any non-manager manual machines exist.
	//
	// We don't bother adding a cleanup for a non state server environment, as
	// RemoveAllEnvironDocs() at the end of apiserver/client.Destroy() removes
	// these documents for us.
	if e.UUID() == e.doc.ServerUUID {
		ops := []txn.Op{e.st.newCleanupOp(cleanupServicesForDyingEnvironment, "")}
		return e.st.runTransaction(ops)
	}
	return nil
}

func (e *Environment) abortDestroy() error {
	// If an environment was added while completing the transaction, rollback
	// the transaction and return an error.
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     e.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"life", Alive}}}},
	}}
	err := e.st.runTransaction(ops)
	if err != nil {
		return errors.Trace(err)
	}

	e.doc.Life = Alive
	return nil
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

// ensureDestroyable returns an error if there is more than one environment and the
// environment to be destroyed is the state server environment.
func (e *Environment) ensureDestroyable() error {
	// after another client checks. Destroy-environment will
	// still fail, but the environment will be in a state where
	// entities can only be destroyed.

	// First, check for manual machines. We bail out if there are any,
	// to stop the user from prematurely hobbling the environment.
	machines, err := e.st.AllMachines()
	if err != nil {
		return errors.Trace(err)
	}

	if err := checkManualMachines(machines); err != nil {
		return errors.Trace(err)
	}

	// If this is not the state server environment, it can be destroyed
	if e.doc.UUID != e.doc.ServerUUID {
		return nil
	}

	environments, closer := e.st.getCollection(environmentsC)
	defer closer()
	n, err := environments.Count()
	if err != nil {
		return errors.Trace(err)
	}
	if n > 1 {
		return errors.Errorf("state server environment cannot be destroyed before all other environments are destroyed")
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
		Owner:      owner.Username(),
		ServerUUID: server,
	}
	return txn.Op{
		C:      environmentsC,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}

// createUniqueOwnerEnvNameOp returns the operation needed to create
// an userenvnameC document with the given owner and environment name.
func createUniqueOwnerEnvNameOp(owner names.UserTag, envName string) txn.Op {
	return txn.Op{
		C:      userenvnameC,
		Id:     userEnvNameIndex(owner.Username(), envName),
		Assert: txn.DocMissing,
		Insert: bson.M{},
	}
}

// assertAliveOp returns a txn.Op that asserts the environment is alive.
func (e *Environment) assertAliveOp() txn.Op {
	return txn.Op{
		C:      environmentsC,
		Id:     e.UUID(),
		Assert: isEnvAliveDoc,
	}
}

// isEnvAlive is an Environment-specific version of isAliveDoc.
//
// Environment documents from versions of Juju prior to 1.17
// do not have the life field; if it does not exist, it should
// be considered to have the value Alive.
var isEnvAliveDoc = bson.D{
	{"life", bson.D{{"$in", []interface{}{Alive, nil}}}},
}
