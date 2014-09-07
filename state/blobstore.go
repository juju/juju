// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/blobstore"
	"gopkg.in/mgo.v2"
)

// getManagedStorage returns a blobstore.ManagedStorage, and an associated
// mgo.Session that must be closed when the user is finished with the
// ManagedStorage.
func (st *State) getManagedStorage(uuid string, session *mgo.Session) blobstore.ManagedStorage {
	rs := blobstore.NewGridFS(blobstoreDB, uuid, session)
	db := st.db.With(session)
	return blobstore.NewManagedStorage(db, rs)
}
