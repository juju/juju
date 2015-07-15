// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/blobstore"
	jujutxn "github.com/juju/txn"
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
	session := st.session.Copy()
	rs := blobstore.NewGridFS(blobstoreDB, uuid, session)
	db := session.DB(jujuDB)
	metadataCollection := db.C(toolsmetadataC)
	txnRunner := jujutxn.NewRunner(jujutxn.RunnerParams{Database: db})
	managedStorage := blobstore.NewManagedStorage(db, rs)
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
