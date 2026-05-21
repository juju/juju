// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	coretrace "github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	objectstoreservice "github.com/juju/juju/domain/objectstore/service"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/s3client"
)

const (
	// States which report the state of the worker.
	stateStarted       = "started"
	stateClientUpdated = "client-updated"
)

// ObjectStoreService provides access to the object store for changes to
// the backend.
type ObjectStoreService interface {
	// GetActiveObjectStoreBackend returns the backend info for the given
	// backend uuid.
	GetActiveObjectStoreBackend(ctx context.Context) (objectstoreservice.BackendInfo, error)

	// WatchObjectStoreBackend returns a watcher that watches the object store
	// backend. The watcher emits the backend changes that either have been
	// added or removed.
	WatchObjectStoreBackend(ctx context.Context) (watcher.StringsWatcher, error)
}

type workerConfig struct {
	ObjectStoreService ObjectStoreService
	HTTPClient         s3client.HTTPClient
	NewClient          NewClientFunc
	Logger             logger.Logger
}

// Validate returns an error if the workerConfig is not valid.
func (cfg workerConfig) Validate() error {
	if cfg.ObjectStoreService == nil {
		return errors.NotValidf("nil ObjectStoreService")
	}
	if cfg.HTTPClient == nil {
		return errors.NotValidf("nil HTTPClient")
	}
	if cfg.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	return nil
}

type s3Worker struct {
	internalStates chan string
	catacomb       catacomb.Catacomb
	config         workerConfig

	mutex   sync.RWMutex
	session objectstore.Session
}

// NewWorker returns a new worker that wraps an S3 Session.
func NewWorker(config workerConfig) (worker.Worker, error) {
	return newWorker(config, nil)
}

// newWorker returns a new worker that wraps an S3 Session. The S3 session
// provides read and write access to the object store. This differs from the
// unit s3 caller which only provides read access.
func newWorker(config workerConfig, internalStates chan string) (*s3Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &s3Worker{
		internalStates: internalStates,
		config:         config,
	}

	// Before we start the catacomb we need to create the initial session.
	client, err := w.makeNewClient(context.Background())
	if err != nil {
		return nil, errors.Trace(err)
	}

	w.session = client

	// Now start the catacomb once we have the initial session.
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "object-store-s3",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

// Session calls the given function with a session. The session is the
// current session held by the worker, which is updated by the loop when
// the backend changes.
//
// The session reference is captured under the lock, but the callback
// executes without holding the lock. This means the session may be
// replaced during the callback's execution. This is safe because S3
// clients are stateless (credentials baked in at creation) and operations
// are idempotent. Callers that get an auth error due to credential
// rotation should retry, which will pick up the new session.
func (w *s3Worker) Session(ctx context.Context, fn func(context.Context, objectstore.Session) error) error {
	ctx, trace := coretrace.Start(ctx, coretrace.NameFromFunc())
	defer trace.End()

	w.mutex.RLock()
	session := w.session
	w.mutex.RUnlock()

	if session == nil {
		return internalerrors.Errorf("no session available").Add(errors.NotSupported)
	}

	return fn(ctx, session)
}

// Kill is part of the worker.Worker interface.
func (w *s3Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *s3Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *s3Worker) loop() (err error) {
	ctx := w.catacomb.Context(context.Background())

	watcher, err := w.config.ObjectStoreService.WatchObjectStoreBackend(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Trace(err)
	}

	// Report the initial started state.
	w.reportInternalState(stateStarted)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			client, err := w.makeNewClient(ctx)
			if err != nil {
				return errors.Trace(err)
			}

			w.mutex.Lock()
			w.session = client
			w.mutex.Unlock()

			w.reportInternalState(stateClientUpdated)
		}
	}
}

func (w *s3Worker) makeNewClient(ctx context.Context) (objectstore.Session, error) {
	// Get the current active backend info. This will include the credentials if
	// the active backend is S3. If the active backend is file, then the
	// credentials will be empty, and the worker will return a noop session.
	backendInfo, err := w.config.ObjectStoreService.GetActiveObjectStoreBackend(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// If the object store type is file, then we don't need to create
	// a new S3 client, just return a noop worker.
	credentials, ok := backendInfo.S3Credentials()
	if !ok {
		return nil, nil
	}

	client, err := w.config.NewClient(
		credentials.Endpoint,
		w.config.HTTPClient,
		s3client.StaticCredentials{
			Key:    credentials.AccessKey,
			Secret: credentials.SecretKey,
		},
		w.config.Logger,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

func (w *s3Worker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}
