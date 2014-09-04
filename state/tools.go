// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
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
	txnRunner, session, closer := st.txnRunner()
	managedStorage := st.getManagedStorage(uuid, session)
	metadataCollection := st.db.With(session).C(toolsmetadataC)
	storage := toolstorageNewStorage(uuid, managedStorage, metadataCollection, txnRunner)
	return &toolsStorageCloser{storage, closer}, nil
}

type toolsStorageCloser struct {
	toolstorage.Storage
	closer closeFunc
}

func (t *toolsStorageCloser) Close() error {
	t.closer()
	return nil
}
