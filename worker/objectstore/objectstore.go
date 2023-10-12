package objectstore

import (
	"context"
	"io"

	"github.com/juju/errors"
	"gopkg.in/tomb.v2"

	coretrace "github.com/juju/juju/core/trace"
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

type stateObjectStoreRequest struct {
	path string
	done chan stateObjectStoreResult
}

type stateObjectStoreResult struct {
	reader io.ReadCloser
	size   int64
	err    error
}

type stateObjectStore struct {
	*baseObjectStore

	tracer  coretrace.Tracer
	session MongoSession

	requests chan stateObjectStoreRequest
}

// NewObjectStoreWorker returns a new object store worker based on the state
// storage.
func NewStateObjectStore(ctx context.Context, namespace string, tracer coretrace.Tracer, mongoSession MongoSession, logger Logger) (TrackedObjectStore, error) {
	s := &stateObjectStore{
		session:  mongoSession,
		tracer:   tracer,
		requests: make(chan stateObjectStoreRequest),
	}

	s.baseObjectStore = newBaseObjectStore(s.loop, namespace, logger)

	return s, nil
}

// Get returns an io.ReadCloser for data at path, namespaced to the
// model.
func (t *stateObjectStore) Get(ctx context.Context, path string) (_ io.ReadCloser, _ int64, err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, t.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("path", path)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	done := make(chan stateObjectStoreResult)

	select {
	case <-t.tomb.Dying():
		return nil, -1, tomb.ErrDying

	case <-ctx.Done():
		return nil, -1, ctx.Err()

	case t.requests <- stateObjectStoreRequest{
		path: path,
		done: done,
	}:
	}

	select {
	case <-t.tomb.Dying():
		return nil, -1, tomb.ErrDying

	case result := <-done:
		return result.reader, result.size, result.err
	}
}

func (t *stateObjectStore) loop() error {
	session := t.session.MongoSession()
	defer func() {
		if session != nil {
			session.Close()
		}
	}()

	store := storage.NewStorage(t.namespace, session)

	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying

		case req := <-t.requests:
			t.logger.Debugf("Requesting object: %v", req.path)

			reader, size, err := store.Get(req.path)
			if err != nil {
				// We got a not found error, we try and be resilient and
				// reconnect to the database.
				if !errors.Is(err, errors.NotFound) {
					if session != nil {
						session.Close()
					}

					session = t.session.MongoSession()
					store = storage.NewStorage(t.namespace, session)
				}
			}

			// Send the result back to the caller.
			select {
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case req.done <- stateObjectStoreResult{
				reader: reader,
				size:   size,
				err:    err,
			}:
			}
		}
	}
}
