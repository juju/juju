// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/state/binarystorage"
)

var binarystorageNew = binarystorage.New

// ToolsStorage returns a new binarystorage.StorageCloser that stores tools
// metadata in the "juju" database "toolsmetadata" collection.
func (st *State) ToolsStorage(store objectstore.ObjectStore) (binarystorage.StorageCloser, error) {
	modelStorage := newBinaryStorageCloser(st.database, store, toolsmetadataC, st.ControllerModelUUID())
	return modelStorage, nil
}

func newBinaryStorageCloser(db Database, store objectstore.ObjectStore, collectionName, uuid string) binarystorage.StorageCloser {
	db, closer1 := db.CopyForModel(uuid)
	metadataCollection, closer2 := db.GetCollection(collectionName)
	txnRunner, closer3 := db.TransactionRunner()

	return &storageCloser{
		Storage: binarystorageNew(store, metadataCollection, txnRunner),
		closer: func() {
			closer3()
			closer2()
			closer1()
		},
	}
}

type storageCloser struct {
	binarystorage.Storage
	closer func()
}

func (sc *storageCloser) Close() error {
	sc.closer()
	return nil
}
