package objectstore

import (
	"context"
	"io"

	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state/storage"
)

type baseObjectStore struct {
	tomb tomb.Tomb

	namespace string
	logger    Logger
}

func newBaseObjectStore(loop func() error, namespace string, logger Logger) *baseObjectStore {
	w := &baseObjectStore{
		namespace: namespace,
		logger:    logger,
	}

	w.tomb.Go(loop)

	return w
}

// Kill implements the worker.Worker interface.
func (s *baseObjectStore) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (s *baseObjectStore) Wait() error {
	return s.tomb.Wait()
}

type stateObjectStore struct {
	*baseObjectStore

	session MongoSession
}

// NewObjectStoreWorker returns a new object store worker based on the state
// storage.
func NewStateObjectStore(ctx context.Context, namespace string, mongoSession MongoSession, logger Logger) (TrackedObjectStore, error) {
	s := &stateObjectStore{
		session: mongoSession,
	}

	s.baseObjectStore = newBaseObjectStore(s.loop, namespace, logger)

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

func (t *stateObjectStore) loop() error {
	<-t.tomb.Dying()
	return tomb.ErrDying
}
