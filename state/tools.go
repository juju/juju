// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/toolstorage"
)

var (
	toolstorageNewStorage = toolstorage.NewStorage
)

// ToolsStorage returns a new toolstorage.StorageCloser
// that stores tools metadata in the "juju" database''
// "toolsmetadata" collection.
func (st *State) ToolsStorage() (toolstorage.StorageCloser, error) {
	environ, err := st.Environment()
	if err != nil {
		return nil, err
	}
	uuid := environ.UUID()
	managedStorage, session := st.getManagedStorage(uuid)
	txnRunner := st.txnRunnerWith(session)
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
