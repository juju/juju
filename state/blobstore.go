// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"io"

	"github.com/juju/blobstore"
	"gopkg.in/mgo.v2"
)

// Storage is an interface providing methods for storing and retrieving
// data by path.
type Storage interface {
	// Get returns an io.ReadCloser for data at path, namespaced to the environment.
	//
	// If the data is still being uploaded and is not fully written yet, a
	// blobstore.ErrUploadPending error is returned. This means the path is valid
	// but the caller should try again later to retrieve the data.
	Get(path string) (r io.ReadCloser, length int64, err error)

	// Put stores data from reader at path, namespaced to the environment.
	Put(path string, r io.Reader, length int64) error

	// Remove removes data at path, namespaced to the environment.
	Remove(path string) error

	// Close closes the storage.
	Close() error
}

// Storage returns a Storage for the environment with the specified UUID.
// The caller must close the storage when it is no longer needed.
func (st *State) Storage() (Storage, error) {
	env, err := st.Environment()
	if err != nil {
		return nil, err
	}
	envUUID := env.UUID()
	session := st.MongoSession().Copy()
	return &storageCloser{
		managedStorage: st.getManagedStorage(envUUID, session),
		envUUID:        envUUID,
		session:        session,
	}, nil
}

// getManagedStorage returns a blobstore.ManagedStorage using the
// specified UUID and mgo.Session.
func (st *State) getManagedStorage(uuid string, session *mgo.Session) blobstore.ManagedStorage {
	rs := blobstore.NewGridFS(blobstoreDB, uuid, session)
	db := st.db.With(session)
	return blobstore.NewManagedStorage(db, rs)
}

type storageCloser struct {
	managedStorage blobstore.ManagedStorage
	envUUID        string
	session        *mgo.Session
}

func (s *storageCloser) Close() error {
	s.session.Close()
	return nil
}

func (s *storageCloser) Get(path string) (r io.ReadCloser, length int64, err error) {
	return s.managedStorage.GetForEnvironment(s.envUUID, path)
}

func (s *storageCloser) Put(path string, r io.Reader, length int64) error {
	return s.managedStorage.PutForEnvironment(s.envUUID, path, r, length)
}

func (s *storageCloser) Remove(path string) error {
	return s.managedStorage.RemoveForEnvironment(s.envUUID, path)
}
