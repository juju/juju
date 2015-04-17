// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/blobstore"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/toolstorage"
)

var (
	toolstorageNewStorage = toolstorage.NewStorage
)

// ToolsStorage returns a new toolstorage.StorageCloser
// that stores tools metadata in the "juju" database''
// "toolsmetadata" collection.
//
// TODO(axw) remove this, add a constructor function in toolstorage.
func (st *State) ToolsStorage() (toolstorage.StorageCloser, error) {
	uuid := st.EnvironUUID()
	session := st.db.Session.Copy()
	txnRunner := st.txnRunner(session)
	rs := blobstore.NewGridFS(blobstoreDB, uuid, session)
	db := st.db.With(session)
	managedStorage := blobstore.NewManagedStorage(db, rs)
	metadataCollection := st.db.With(session).C(toolsmetadataC)
	storage := toolstorageNewStorage(uuid, managedStorage, metadataCollection, txnRunner)
	return &toolsStorageCloser{storage, session}, nil
}

type toolsStorageCloser struct {
	toolstorage.Storage
	session *mgo.Session
}

func (t *toolsStorageCloser) Close() error {
	t.session.Close()
	return nil
}
