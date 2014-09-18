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
	// Get returns an io.ReadCloser for data at path, namespaced to the
	// environment.
	//
	// If the data is still being uploaded and is not fully written yet, a
	// blobstore.ErrUploadPending error is returned. This means the path is
	// valid but the caller should try again later to retrieve the data.
	Get(path string) (r io.ReadCloser, length int64, err error)

	// Put stores data from reader at path, namespaced to the environment.
	Put(path string, r io.Reader, length int64) error

	// Remove removes data at path, namespaced to the environment.
	Remove(path string) error
}

// Storage returns a Storage for the environment with the specified UUID.
// The caller must close the storage when it is no longer needed.
func (st *State) Storage() Storage {
	return stateStorage{st}
}

// getManagedStorage returns a blobstore.ManagedStorage using the
// specified UUID and mgo.Session.
func (st *State) getManagedStorage(uuid string, session *mgo.Session) blobstore.ManagedStorage {
	rs := blobstore.NewGridFS(blobstoreDB, uuid, session)
	db := st.db.With(session)
	return blobstore.NewManagedStorage(db, rs)
}

type stateStorage struct {
	st *State
}

func (s stateStorage) blobstore() (uuid string, session *mgo.Session, ms blobstore.ManagedStorage, err error) {
	env, err := s.st.Environment()
	if err != nil {
		return "", nil, nil, err
	}
	uuid = env.UUID()
	session = s.st.MongoSession().Copy()
	return uuid, session, s.st.getManagedStorage(uuid, session), nil
}

func (s stateStorage) Get(path string) (r io.ReadCloser, length int64, err error) {
	uuid, session, ms, err := s.blobstore()
	if err != nil {
		return nil, -1, err
	}
	r, length, err = ms.GetForEnvironment(uuid, path)
	if err != nil {
		session.Close()
		return nil, -1, err
	}
	return &stateStorageReadCloser{r, session}, length, nil
}

func (s stateStorage) Put(path string, r io.Reader, length int64) error {
	uuid, session, ms, err := s.blobstore()
	if err != nil {
		return err
	}
	defer session.Close()
	return ms.PutForEnvironment(uuid, path, r, length)
}

func (s stateStorage) Remove(path string) error {
	uuid, session, ms, err := s.blobstore()
	if err != nil {
		return err
	}
	defer session.Close()
	return ms.RemoveForEnvironment(uuid, path)
}

type stateStorageReadCloser struct {
	io.ReadCloser
	session *mgo.Session
}

func (r *stateStorageReadCloser) Close() error {
	r.session.Close()
	return r.ReadCloser.Close()
}
