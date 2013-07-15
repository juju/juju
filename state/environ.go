// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/errors"
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
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	doc := environmentDoc{}
	err := st.environments.Find(D{{"uuid", D{{"$ne", ""}}}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, errors.NotFoundf("environment")
	} else if err != nil {
		return nil, err
	}
	env := &Environment{
		st:  st,
		doc: doc,
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
func (e Environment) Tag() string {
	return "environment-" + e.doc.Name
}

// UUID returns the universally unique identifier of the environment.
func (e Environment) UUID() string {
	return e.doc.UUID
}

// globalKey returns the global database key for the environment.
func (e *Environment) globalKey() string {
	return environGlobalKey
}

// createEnvironmentOp returns the operation needed to create
// an environment document with the given name and UUID.
func createEnvironmentOp(st *State, name, uuid string) txn.Op {
	doc := &environmentDoc{uuid, name}
	return txn.Op{
		C:      st.environments.Name,
		Id:     uuid,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}
