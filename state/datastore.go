// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import "gopkg.in/mgo.v2/bson"

// datastoreDoc represents storage associated with a unit.
type datastoreDoc struct {
	Id     bson.ObjectId `bson:"_id"`
	UnitId string        `bson:"unitid"`
	Name   string        `bson:"name"`
	Type   string        `bson:"type"`
	Path   string        `bson:"path"`

	// TODO(axw) storage specification
}
