// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"

	"github.com/juju/blobstore"
	"gopkg.in/mgo.v2"
)

const (
	// metadataDB is the name of the blobstore metadata database.
	metadataDB = "juju"

	// blobstoreDB is the name of the blobstore GridFS database.
	blobstoreDB = "blobstore"
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
func NewStorage(envUUID string, session *mgo.Session) Storage {
	return stateStorage{envUUID, session}
}

type stateStorage struct {
	envUUID string
	session *mgo.Session
}

func (s stateStorage) blobstore() (*mgo.Session, blobstore.ManagedStorage) {
	session := s.session.Copy()
	rs := blobstore.NewGridFS(blobstoreDB, s.envUUID, session)
	db := session.DB(metadataDB)
	return session, blobstore.NewManagedStorage(db, rs)
}

func (s stateStorage) Get(path string) (r io.ReadCloser, length int64, err error) {
	session, ms := s.blobstore()
	r, length, err = ms.GetForEnvironment(s.envUUID, path)
	if err != nil {
		session.Close()
		return nil, -1, err
	}
	return &stateStorageReadCloser{r, session}, length, nil
}

func (s stateStorage) Put(path string, r io.Reader, length int64) error {
	session, ms := s.blobstore()
	defer session.Close()
	return ms.PutForEnvironment(s.envUUID, path, r, length)
}

func (s stateStorage) Remove(path string) error {
	session, ms := s.blobstore()
	defer session.Close()
	return ms.RemoveForEnvironment(s.envUUID, path)
}

type stateStorageReadCloser struct {
	io.ReadCloser
	session *mgo.Session
}

func (r *stateStorageReadCloser) Close() error {
	r.session.Close()
	return r.ReadCloser.Close()
}
