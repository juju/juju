// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"

	"github.com/juju/juju/core/life"
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

var (
	isAliveDoc = bson.D{{"life", Alive}}
	isDyingDoc = bson.D{{"life", Dying}}
	isDeadDoc  = bson.D{{"life", Dead}}
	notDeadDoc = bson.D{{"life", bson.D{{"$ne", Dead}}}}

	errDeadOrGone     = errors.New("neither alive nor dying")
	errAlreadyDying   = errors.New("already dying")
	errAlreadyRemoved = errors.New("already removed")
)

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

func checkLifeWithSession(coll mongo.Collection, id interface{}, sel bson.DocElem) (bool, error) {
	n, err := coll.Find(bson.D{{"_id", id}, sel}).Count()
	return n == 1, err
}
