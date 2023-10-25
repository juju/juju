// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"io"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state/storage"
)

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
	store := storage.NewStorage(t.namespace, session)
	return store.Get(path)
}

// Put stores data from reader at path, namespaced to the model.
func (t *stateObjectStore) Put(ctx context.Context, path string, r io.Reader, size int64) error {
	session := t.session.MongoSession()
	store := storage.NewStorage(t.namespace, session)
	return store.Put(path, r, size)
}

// Remove removes data at path, namespaced to the model.
func (t *stateObjectStore) Remove(ctx context.Context, path string) error {
	session := t.session.MongoSession()
	store := storage.NewStorage(t.namespace, session)
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
