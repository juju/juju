package objectstore

import (
	"context"

	"gopkg.in/tomb.v2"
)

type objectStore struct {
	tomb tomb.Tomb

	namespace string
	logger    Logger
}

// NewObjectStoreWorker returns a new object store worker.
func NewObjectStoreWorker(ctx context.Context, namespace string, logger Logger) (TrackedObjectStore, error) {
	s := &objectStore{
		namespace: namespace,
		logger:    logger,
	}

	s.tomb.Go(s.loop)

	return s, nil
}

// Kill implements the worker.Worker interface.
func (s *objectStore) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements the worker.Worker interface.
func (s *objectStore) Wait() error {
	return s.tomb.Wait()
}

func (t *objectStore) loop() error {
	<-t.tomb.Dying()
	return tomb.ErrDying
}
