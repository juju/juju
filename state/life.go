// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"

	"launchpad.net/juju-core/state/api/params"
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

var lifeStrings = [nLife]params.Life{
	Alive: params.Alive,
	Dying: params.Dying,
	Dead:  params.Dead,
}

func (l Life) String() string {
	return string(lifeStrings[l])
}

var isAliveDoc = bson.D{{"life", Alive}}
var isDeadDoc = bson.D{{"life", Dead}}
var notDeadDoc = bson.D{{"life", bson.D{{"$ne", Dead}}}}

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

func isAlive(coll *mgo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(bson.D{{"_id", id}, {"life", Alive}}).Count()
	return n == 1, err
}

func isNotDead(coll *mgo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(bson.D{{"_id", id}, {"life", bson.D{{"$ne", Dead}}}}).Count()
	return n == 1, err
}
