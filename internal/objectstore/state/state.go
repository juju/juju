// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"io"

	"github.com/juju/mgo/v3"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state/storage"
)

<<<<<<< HEAD:worker/objectstore/objectstore.go
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
=======
// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

// MongoSession is the interface that is used to get a mongo session.
// Deprecated: is only here for backwards compatibility.
type MongoSession interface {
	MongoSession() *mgo.Session
}

type stateObjectStore struct {
	tomb tomb.Tomb

	namespace string

	session MongoSession
	logger  Logger
}

// New returns a new object store worker based on the state
// storage.
func New(namespace string, mongoSession MongoSession, logger Logger) (*stateObjectStore, error) {
	s := &stateObjectStore{
		session:   mongoSession,
		namespace: namespace,
		logger:    logger,
	}

	s.tomb.Go(func() error {
		<-s.tomb.Dying()
		return tomb.ErrDying
	})
>>>>>>> 8f855838db (File store backend):internal/objectstore/state/state.go

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
<<<<<<< HEAD:worker/objectstore/objectstore.go
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
=======
func (t *stateObjectStore) Kill() {
	t.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (t *stateObjectStore) Wait() error {
	return t.tomb.Wait()
>>>>>>> 8f855838db (File store backend):internal/objectstore/state/state.go
}
