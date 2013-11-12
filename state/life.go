// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"labix.org/v2/mgo"

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

var isAliveDoc = D{{"life", Alive}}
var isDeadDoc = D{{"life", Dead}}
var notDeadDoc = D{{"life", D{{"$ne", Dead}}}}

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
	n, err := coll.Find(D{{"_id", id}, {"life", Alive}}).Count()
	return n == 1, err
}

func isNotDead(coll *mgo.Collection, id interface{}) (bool, error) {
	n, err := coll.Find(D{{"_id", id}, {"life", D{{"$ne", Dead}}}}).Count()
	return n == 1, err
}
