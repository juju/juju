// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagestorage

import (
	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"
)

// ManagedStorage returns the managedStorage attribute for the storage.
func ManagedStorage(s Storage) blobstore.ManagedStorage {
	return s.(*imageStorage).managedStorage
}

// MetadataCollection returns the metadataCollection attribute for the storage.
func MetadataCollection(s Storage) *mgo.Collection {
	return s.(*imageStorage).metadataCollection
}

// SetRemoveFailsManagedStorage sets a patched managedStorage attribute for storage,
// which fails when Remove is called.
func SetRemoveFailsManagedStorage(s Storage) {
	s.(*imageStorage).managedStorage = removeFailsManagedStorage{s.(*imageStorage).managedStorage}
}

type removeFailsManagedStorage struct {
	blobstore.ManagedStorage
}

func (removeFailsManagedStorage) RemoveForEnvironment(uuid, path string) error {
	return errors.Errorf("cannot remove %s:%s", uuid, path)
}

var TxnRunner = &txnRunner
