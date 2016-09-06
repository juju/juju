// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/blobstore.v2"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state/binarystorage"
)

var binarystorageNew = binarystorage.New

// ToolsStorage returns a new binarystorage.StorageCloser that stores tools
// metadata in the "juju" database "toolsmetadata" collection.
func (st *State) ToolsStorage() (binarystorage.StorageCloser, error) {
	if st.IsController() {
		return st.newBinaryStorageCloser(toolsmetadataC, st.ModelUUID()), nil
	}

	// This is a hosted model. Hosted models have their own tools
	// catalogue, which we combine with the controller's.

	controllerModel, err := st.ControllerModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerSt, err := st.ForModel(controllerModel.ModelTag())
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer controllerSt.Close()
	controllerStorage, err := controllerSt.ToolsStorage()
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelStorage := st.newBinaryStorageCloser(toolsmetadataC, st.ModelUUID())
	storage, err := binarystorage.NewLayeredStorage(modelStorage, controllerStorage)
	if err != nil {
		modelStorage.Close()
		controllerStorage.Close()
		return nil, errors.Trace(err)
	}
	return &storageCloser{storage, func() {
		modelStorage.Close()
		controllerStorage.Close()
	}}, nil
}

// GUIStorage returns a new binarystorage.StorageCloser that stores GUI archive
// metadata in the "juju" database "guimetadata" collection.
func (st *State) GUIStorage() (binarystorage.StorageCloser, error) {
	controllerModel, err := st.ControllerModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st.newBinaryStorageCloser(guimetadataC, controllerModel.UUID()), nil
}

func (st *State) newBinaryStorageCloser(collectionName, uuid string) binarystorage.StorageCloser {
	db, closer1 := st.database.Copy()
	metadataCollection, closer2 := db.GetCollection(collectionName)
	txnRunner, closer3 := db.TransactionRunner()
	closer := func() {
		closer3()
		closer2()
		closer1()
	}
	storage := newBinaryStorage(uuid, metadataCollection, txnRunner)
	return &storageCloser{storage, closer}
}

func newBinaryStorage(uuid string, metadataCollection mongo.Collection, txnRunner jujutxn.Runner) binarystorage.Storage {
	db := metadataCollection.Writeable().Underlying().Database
	rs := blobstore.NewGridFS(blobstoreDB, blobstoreDB, db.Session)
	managedStorage := blobstore.NewManagedStorage(db, rs)
	return binarystorageNew(uuid, managedStorage, metadataCollection, txnRunner)
}

type storageCloser struct {
	binarystorage.Storage
	closer func()
}

func (sc *storageCloser) Close() error {
	sc.closer()
	return nil
}
