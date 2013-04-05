package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"

	"launchpad.net/juju-core/trivial"
)

// environGlobalKey is the key for the environment, its
// settings and constraints.
const environGlobalKey = "e"

// Environment represents the state of an environment.
type Environment struct {
	st   *State
	name string
	uuid trivial.UUID
	annotator
}

// environmentDoc represents the internal state of the environment in MongoDB.
type environmentDoc struct {
	UUID trivial.UUID
	Name string
}

// Environment returns the environment entity.
func (st *State) Environment() (*Environment, error) {
	doc := environmentDoc{}
	err := st.environments.Find(D{{"name", D{{"$ne", ""}}}}).One(&doc)
	if err == mgo.ErrNotFound {
		return nil, NotFoundf("environment")
	}
	env := &Environment{
		st:   st,
		name: doc.Name,
		uuid: doc.UUID,
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
	return "environment-" + e.name
}

// UUID returns the universally unique identifier of the environment.
func (e Environment) UUID() trivial.UUID {
	return e.uuid.Copy()
}

// globalKey returns the global database key for the environment.
func (e *Environment) globalKey() string {
	return environGlobalKey
}

// createEnvironmentOp returns the operation needed to create
// an environment document with the given name and UUID.
func createEnvironmentOp(st *State, name string, uuid trivial.UUID) txn.Op {
	doc := &environmentDoc{
		UUID: uuid,
		Name: name,
	}
	return txn.Op{
		C:      st.environments.Name,
		Id:     environGlobalKey,
		Assert: txn.DocMissing,
		Insert: doc,
	}
}
