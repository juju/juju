package mstate

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
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

// ensureLife changes the lifecycle state of the entity with
// the id in the collection.
func ensureLife(st *State, coll *mgo.Collection, id interface{}, life Life, descr string) error {
	if life == Alive {
		panic("cannot set life to alive")
	}
	sel := bson.D{
		{"_id", id},
		// $lte is used so that we don't overwrite a previous
		// change we don't know about. 
		{"life", bson.D{{"$lte", life}}},
	}
	change := bson.D{{"$set", bson.D{{"life", life}}}}
	ops := []txn.Op{{
		C:      coll.Name,
		Id:     id,
		Assert: sel,
		Update: change,
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
