// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourcestorage

import (
	"github.com/juju/blobstore"
	"github.com/juju/errors"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/charmresources"
)

var NewResourceManagerInternal = newResourceManagerInternal

type ResourceMetadataDoc resourceMetadataDoc

// ManagedStorage returns the managedStorage attribute for the storage.
func ManagedStorage(r charmresources.ResourceManager, session *mgo.Session) blobstore.ManagedStorage {
	return r.(*resourceStorage).getManagedStorage(session)
}

// MetadataCollection returns the metadataCollection attribute for the storage.
func MetadataCollection(r charmresources.ResourceManager) *mgo.Collection {
	return r.(*resourceStorage).metadataCollection
}

// RemoveFailsManagedStorage returns a patched managedStorage,
// which fails when Remove is called.
func RemoveFailsManagedStorage(db *mgo.Database, rs blobstore.ResourceStorage) blobstore.ManagedStorage {
	return removeFailsManagedStorage{blobstore.NewManagedStorage(db, rs)}
}

type removeFailsManagedStorage struct {
	blobstore.ManagedStorage
}

func (removeFailsManagedStorage) RemoveForEnvironment(uuid, path string) error {
	return errors.Errorf("cannot remove %s:%s", uuid, path)
}
