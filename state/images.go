// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/state/imagestorage"
)

var (
	imageStorageNewStorage = imagestorage.NewStorage
)

// ImageStorage returns a new imagestorage.StorageCloser
// that stores image metadata in the "juju" database
// "imagemetadata" collection.
func (st *State) ImageStorage() (imagestorage.StorageCloser, error) {
	environ, err := st.Environment()
	if err != nil {
		return nil, err
	}
	uuid := environ.UUID()

	session := st.db.Session.Copy()
	txnRunner := st.txnRunner(session)
	managedStorage := st.getManagedStorage(uuid, session)
	metadataCollection := st.db.With(session).C(imagemetadataC)
	storage := imageStorageNewStorage(uuid, managedStorage, metadataCollection, txnRunner)
	return &imageStorageCloser{storage, session}, nil
}

type imageStorageCloser struct {
	imagestorage.Storage
	session *mgo.Session
}

func (t *imageStorageCloser) Close() error {
	t.session.Close()
	return nil
}
