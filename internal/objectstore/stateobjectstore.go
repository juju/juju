// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"

	"github.com/juju/mgo/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/internal/objectstore/state"
)

// MongoSession is the interface that is used to get a mongo session.
// Deprecated: is only here for backwards compatibility.
type MongoSession interface {
	MongoSession() *mgo.Session
}

type stateObjectStore struct {
	tomb      tomb.Tomb
	namespace string
	logger    Logger

	session MongoSession
}

// NewObjectStoreWorker returns a new object store worker based on the state
// storage.
func NewStateObjectStore(ctx context.Context, namespace string, mongoSession MongoSession, logger Logger) (TrackedObjectStore, error) {
	s := &stateObjectStore{
		namespace: namespace,
		session:   mongoSession,
		logger:    logger,
	}

	s.tomb.Go(s.loop)

	return s, nil
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *stateObjectStore) Get(ctx context.Context, path string) (io.ReadCloser, int64, error) {
	session := t.session.MongoSession()
	store := state.NewStorage(t.namespace, session)
	return store.Get(path)
}

// Put stores data from reader at path, namespaced to the model.
func (t *stateObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) error {
	session := t.session.MongoSession()
	store := state.NewStorage(t.namespace, session)
	return store.Put(path, r, size)
}

// Put stores data from reader at path, namespaced to the model.
// It also ensures the stored data has the correct hash.
func (t *stateObjectStore) PutAndCheckHash(ctx context.Context, path string, r io.Reader, size int64, hash string) error {
	session := t.session.MongoSession()
	store := state.NewStorage(t.namespace, session)
	return store.PutAndCheckHash(path, r, size, hash)
}

// Remove removes data at path, namespaced to the model.
func (t *stateObjectStore) Remove(ctx context.Context, path string) error {
	session := t.session.MongoSession()
	store := state.NewStorage(t.namespace, session)
	return store.Remove(path)
}

// Kill implements the worker.Worker interface.
func (s *stateObjectStore) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (s *stateObjectStore) Wait() error {
	return s.tomb.Wait()
}

func (t *stateObjectStore) loop() error {
	<-t.tomb.Dying()
	return tomb.ErrDying
}
