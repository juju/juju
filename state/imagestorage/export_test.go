// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagestorage

import (
	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
)

// ManagedStorage returns the managedStorage attribute for the storage.
func ManagedStorage(s Storage, session *mgo.Session) blobstore.ManagedStorage {
	return s.(*imageStorage).getManagedStorage(session)
}

// MetadataCollection returns the metadataCollection attribute for the storage.
func MetadataCollection(s Storage) *mgo.Collection {
	return s.(*imageStorage).metadataCollection
}

// RemoveFailsManagedStorage returns a patched managedStorage,
// which fails when Remove is called.
var RemoveFailsManagedStorage = func(session *mgo.Session) blobstore.ManagedStorage {
	rs := blobstore.NewGridFS(ImagesDB, ImagesDB, session)
	db := session.DB(ImagesDB)
	metadataDb := db.With(session)
	return removeFailsManagedStorage{blobstore.NewManagedStorage(metadataDb, rs)}
}

type removeFailsManagedStorage struct {
	blobstore.ManagedStorage
}

func (removeFailsManagedStorage) RemoveForEnvironment(uuid, path string) error {
	return errors.Errorf("cannot remove %s:%s", uuid, path)
}

var TxnRunner = &txnRunner
var GetManagedStorage = &getManagedStorage
