// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/mongo"
)

// Life represents the lifecycle state of the entities
// Relation, Unit, Service and Machine.
type Life int8

const (
	Alive Life = iota
	Dying
	Dead
)

func (l Life) String() string {
	switch l {
	case Alive:
		return "alive"
	case Dying:
		return "dying"
	case Dead:
		return "dead"
	default:
		return "unknown"
	}
}

var (
	isAliveDoc = bson.D{{"life", Alive}}
	isDyingDoc = bson.D{{"life", Dying}}
	isDeadDoc  = bson.D{{"life", Dead}}
	notDeadDoc = bson.D{{"life", bson.D{{"$ne", Dead}}}}

	errDeadOrGone     = errors.New("neither alive nor dying")
	errAlreadyDying   = errors.New("already dying")
	errAlreadyDead    = errors.New("already dead")
	errAlreadyRemoved = errors.New("already removed")
	errNotDying       = errors.New("not dying")
)

// Living describes state entities with a lifecycle.
type Living interface {
	Life() Life
	Destroy() error
	Refresh() error
}

// AgentLiving describes state entities with a lifecycle and an agent that
// manages it.
type AgentLiving interface {
	Living
	EnsureDead() error
	Remove() error
}

func isAlive(st *State, collName string, id interface{}) (bool, error) {
	coll, closer := st.getCollection(collName)
	defer closer()
	return isAliveWithSession(coll, id)
}

func isAliveWithSession(coll mongo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(bson.D{{"_id", id}, {"life", Alive}}).Count()
	return n == 1, err
}

func isNotDead(st *State, collName string, id interface{}) (bool, error) {
	coll, closer := st.getCollection(collName)
	defer closer()
	return isNotDeadWithSession(coll, id)
}

func isNotDeadWithSession(coll mongo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(bson.D{{"_id", id}, {"life", bson.D{{"$ne", Dead}}}}).Count()
	return n == 1, err
}
