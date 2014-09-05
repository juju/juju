// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/mgo.v2/txn"
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
	UUID string `bson:"_id"`
	Name string
	Life Life
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	environments, closer := st.getCollection(environmentsC)
	defer closer()

	env := &Environment{st: st}
	if err := env.refresh(environments.Find(nil)); err != nil {
		return nil, err
	}
	env.annotator = annotator{
		globalKey: env.globalKey(),
		tag:       env.Tag(),
		st:        st,
	}
	return env, nil
}

// Tag returns a name identifying the environment.
// The returned name will be different from other Tag values returned
// by any other entities from the same state.
func (e *Environment) Tag() names.Tag {
	return names.NewEnvironTag(e.doc.UUID)
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
	// For now, just returns "admin".
	return names.NewUserTag(AdminUser)
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

// createEnvironmentOp returns the operation needed to create
// an environment document with the given name and UUID.
func createEnvironmentOp(st *State, name, uuid string) txn.Op {
	doc := &environmentDoc{uuid, name, Alive}
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

// isEnvAlive is an Environment-specific versio nof isAliveDoc.
//
// Environment documents from versions of Juju prior to 1.17
// do not have the life field; if it does not exist, it should
// be considered to have the value Alive.
var isEnvAliveDoc = bson.D{
	{"life", bson.D{{"$in", []interface{}{Alive, nil}}}},
}
