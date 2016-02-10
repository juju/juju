// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"io"

	"gopkg.in/juju/blobstore.v2"
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
	// model.
	//
	// If the data is still being uploaded and is not fully written yet, a
	// blobstore.ErrUploadPending error is returned. This means the path is
	// valid but the caller should try again later to retrieve the data.
	Get(path string) (r io.ReadCloser, length int64, err error)

	// Put stores data from reader at path, namespaced to the model.
	Put(path string, r io.Reader, length int64) error

	// PutAndCheckHash stores data from reader at path, namespaced to
	// the model. It also ensures the stored data has the correct
	// hash.
	PutAndCheckHash(path string, r io.Reader, length int64, hash string) error

	// Remove removes data at path, namespaced to the model.
	Remove(path string) error
}

// Storage returns a Storage for the model with the specified UUID.
func NewStorage(modelUUID string, session *mgo.Session) Storage {
	return stateStorage{modelUUID, session}
}

type stateStorage struct {
	modelUUID string
	session   *mgo.Session
}

func (s stateStorage) blobstore() (*mgo.Session, blobstore.ManagedStorage) {
	session := s.session.Copy()
	rs := blobstore.NewGridFS(blobstoreDB, s.modelUUID, session)
	db := session.DB(metadataDB)
	return session, blobstore.NewManagedStorage(db, rs)
}

func (s stateStorage) Get(path string) (r io.ReadCloser, length int64, err error) {
	session, ms := s.blobstore()
	r, length, err = ms.GetForBucket(s.modelUUID, path)
	if err != nil {
		session.Close()
		return nil, -1, err
	}
	return &stateStorageReadCloser{r, session}, length, nil
}

func (s stateStorage) Put(path string, r io.Reader, length int64) error {
	session, ms := s.blobstore()
	defer session.Close()
	return ms.PutForBucket(s.modelUUID, path, r, length)
}

func (s stateStorage) PutAndCheckHash(path string, r io.Reader, length int64, hash string) error {
	session, ms := s.blobstore()
	defer session.Close()
	return ms.PutForBucketAndCheckHash(s.modelUUID, path, r, length, hash)
}

func (s stateStorage) Remove(path string) error {
	session, ms := s.blobstore()
	defer session.Close()
	return ms.RemoveForBucket(s.modelUUID, path)
}

type stateStorageReadCloser struct {
	io.ReadCloser
	session *mgo.Session
}

func (r *stateStorageReadCloser) Close() error {
	r.session.Close()
	return r.ReadCloser.Close()
}
