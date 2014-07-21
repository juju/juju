// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import "labix.org/v2/mgo"

// CollectionFromName returns a named collection on the specified database,
// initialised with a new session. Also returned is a close function which is
// called when the collection is no longer required.
func CollectionFromName(db *mgo.Database, coll string) (*mgo.Collection, func()) {
	session := db.Session.Copy()
	newColl := db.C(coll).With(session)
	return newColl, session.Close
}
