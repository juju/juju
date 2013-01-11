package state

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/txn"
	"launchpad.net/juju-core/trivial"
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

var notDeadDoc = D{{"life", D{{"$ne", Dead}}}}
var isAliveDoc = D{{"life", Alive}}

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
	EnsureDying() error
	EnsureDead() error
	Refresh() error
}

// ensureDying advances the specified entity's life status to Dying, if necessary.
func ensureDying(st *State, coll *mgo.Collection, id interface{}, desc string) error {
	ops := []txn.Op{{
		C:      coll.Name,
		Id:     id,
		Assert: isAliveDoc,
		Update: D{{"$set", D{{"life", Dying}}}},
	}}
	if err := st.runner.Run(ops, "", nil); err == txn.ErrAborted {
		return nil
	} else if err != nil {
		return fmt.Errorf("cannot start termination of %s %#v: %v", desc, id, err)
	}
	return nil
}

// ensureDead advances the specified entity's life status to Dead, if necessary.
// Preconditions can be supplied in assertOps; if the preconditions fail, the error
// will contain assertMsg. If the entity is not found, no error is returned.
func ensureDead(st *State, coll *mgo.Collection, id interface{}, desc string, assertOps []txn.Op, assertMsg string) (err error) {
	defer trivial.ErrorContextf(&err, "cannot finish termination of %s %#v", desc, id)
	ops := append(assertOps, txn.Op{
		C:      coll.Name,
		Id:     id,
		Update: D{{"$set", D{{"life", Dead}}}},
	})
	if err = st.runner.Run(ops, "", nil); err == nil {
		return nil
	} else if err != txn.ErrAborted {
		return err
	}
	var doc struct{ Life }
	if err = coll.FindId(id).One(&doc); err == mgo.ErrNotFound {
		return nil
	} else if err != nil {
		return err
	} else if doc.Life != Dead {
		return fmt.Errorf(assertMsg)
	}
	return nil
}

func isAlive(coll *mgo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(D{{"_id", id}, {"life", Alive}}).Count()
	return n == 1, err
}

func isNotDead(coll *mgo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(D{{"_id", id}, {"life", D{{"$ne", Dead}}}}).Count()
	return n == 1, err
}
