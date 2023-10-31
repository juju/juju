// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/mongo"
)

// Life represents the lifecycle state of the entities
// Relation, Unit, Application and Machine.
type Life int8

const (
	Alive Life = iota
	Dying
	Dead
)

// String is deprecated, use Value.
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

// Value returns the core.life.Value type.
func (l Life) Value() life.Value {
	switch l {
	case Alive:
		return life.Alive
	case Dying:
		return life.Dying
	case Dead:
		return life.Dead
	default:
		return life.Value("unknown")
	}
}

// LifeFromValue converts a life.Value into it's corresponding
// state.Life
func LifeFromValue(value life.Value) Life {
	return valueMap[value]
}

var (
	isAliveDoc = bson.D{{"life", Alive}}
	isDyingDoc = bson.D{{"life", Dying}}
	isDeadDoc  = bson.D{{"life", Dead}}
	notDeadDoc = bson.D{{"life", bson.D{{"$ne", Dead}}}}

	errDeadOrGone     = errors.New("neither alive nor dying")
	errAlreadyDying   = errors.New("already dying")
	errAlreadyRemoved = errors.New("already removed")
)

var valueMap = map[life.Value]Life{
	life.Alive: Alive,
	life.Dying: Dying,
	life.Dead:  Dead,
}

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
	Remove(objectstore.ObjectStore) error
}

func isAlive(mb modelBackend, collName string, id interface{}) (bool, error) {
	coll, closer := mb.db().GetCollection(collName)
	defer closer()
	return isAliveWithSession(coll, id)
}

func isAliveWithSession(coll mongo.Collection, id interface{}) (bool, error) {
	return checkLifeWithSession(coll, id, bson.DocElem{"life", Alive})
}

func isNotDead(mb modelBackend, collName string, id interface{}) (bool, error) {
	coll, closer := mb.db().GetCollection(collName)
	defer closer()
	return isNotDeadWithSession(coll, id)
}

func isNotDeadWithSession(coll mongo.Collection, id interface{}) (bool, error) {
	return checkLifeWithSession(coll, id, bson.DocElem{"life", bson.D{{"$ne", Dead}}})
}

func isDying(mb modelBackend, collName string, id interface{}) (bool, error) {
	coll, closer := mb.db().GetCollection(collName)
	defer closer()
	return isDyingWithSession(coll, id)
}

func isDyingWithSession(coll mongo.Collection, id interface{}) (bool, error) {
	return checkLifeWithSession(coll, id, bson.DocElem{"life", Dying})
}

func checkLifeWithSession(coll mongo.Collection, id interface{}, sel bson.DocElem) (bool, error) {
	n, err := coll.Find(bson.D{{"_id", id}, sel}).Count()
	return n == 1, err
}
