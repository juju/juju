package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
)

// Life represents the lifecycle state of the entities
// Relation, Unit, Service and Machine.
type Life int8

const (
	Alive Life = iota
	Dying
	Dead
	nLife
)

var notDead = D{{"life", D{{"$ne", Dead}}}}
var isAlive = D{{"life", Alive}}

var lifeStrings = [nLife]string{
	Alive: "alive",
	Dying: "dying",
	Dead:  "dead",
}

func (l Life) String() string {
	return lifeStrings[l]
}

// Living describes state entities with a lifecycle.
type Living interface {
	Life() Life
	Kill() error
	Die() error
	Refresh() error
}

// ensureLife advances the lifecycle state of the entity with the given
// id in the given collection, if necessary.  Life specifies the desired
// life state, which cannot be Alive.
func ensureLife(st *State, coll *mgo.Collection, id interface{}, life Life, descr string) error {
	if life == Alive {
		panic("cannot set life to alive")
	}
	sel := D{
		// $lte is used so that we don't overwrite a previous
		// change we don't know about. 
		{"life", D{{"$lte", life}}},
	}
	ops := []txn.Op{{
		C:      coll.Name,
		Id:     id,
		Assert: sel,
		Update: D{{"$set", D{{"life", life}}}},
	}}
	err := st.runner.Run(ops, "", nil)
	if err == txn.ErrAborted {
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot set life to %q for %s %q: %v", life, descr, id, err)
	}
	return nil
}
