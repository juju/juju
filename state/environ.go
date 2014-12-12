// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/juju/mongo"
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
	annotator
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
	env.annotator = annotator{
		globalKey: environGlobalKey,
		tag:       env.Tag(),
		st:        st,
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
	env.annotator = annotator{
		globalKey: environGlobalKey,
		tag:       env.Tag(),
		st:        st,
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
	env.annotator = annotator{
		globalKey: environGlobalKey,
		tag:       env.Tag(),
		st:        st,
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
	ssEnv, err := st.StateServerEnvironment()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not load state server environment")
	}

	newState, err := open(st.mongoInfo, mongo.DialOpts{}, st.policy)
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not create state for new environment")
	}
	defer func() {
		if err != nil {
			newState.Close()
		}
	}()

	ops, err := newState.envSetupOps(cfg, ssEnv.UUID(), owner)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to create new environment")
	}
	err = newState.runTransaction(ops)
	if err == txn.ErrAborted {
		err = errors.New("environment already exists")
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	newEnv, err := newState.Environment()
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not load new environment")
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
func (e *Environment) Destroy() error {
	if err := e.ensureDestroyable(); err != nil {
		return err
	}

	if e.Life() != Alive {
		return nil
	}
	// TODO(axw) 2013-12-11 #1218688
	// Resolve the race between checking for manual machines and
	// destroying the environment. We can set Environment to Dying
	// to resolve the race, but that puts the environment into an
	// unusable state.
	//
	// We add a cleanup for services, but not for machines;
	// machines are destroyed via the provider interface. The
	// exception to this rule is manual machines; the API prevents
	// destroy-environment from succeeding if any non-manager
	// manual machines exist.
	ops := []txn.Op{{
		C:      environmentsC,
		Id:     e.doc.UUID,
		Update: bson.D{{"$set", bson.D{{"life", Dying}}}},
		Assert: isEnvAliveDoc,
	}, e.st.newCleanupOp(cleanupServicesForDyingEnvironment, "")}
	err := e.st.runTransaction(ops)
	switch err {
	case nil, txn.ErrAborted:
		// If the transaction aborted, the environment is either
		// Dying or Dead; neither case is an error. If it's Dead,
		// reporting it as Dying is not incorrect; the user thought
		// it was Alive, so we've progressed towards Dead. If the
		// user then calls Refresh they'll get the true value.
		e.doc.Life = Dying
	}
	return err
}

// ensureDestroyable returns an error if there is more than one environment and the
// environment to be destroyed is the state server environment.
func (e *Environment) ensureDestroyable() error {
	ssinfo, err := e.st.StateServerInfo()
	if err != nil {
		return errors.Annotate(err, "could not get state server info")
	}
	ssuuid := ssinfo.EnvironmentTag.Id()

	// This is not the state server environment, so it can be destroyed
	if ssuuid != e.UUID() {
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
