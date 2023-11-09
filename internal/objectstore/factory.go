// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/core/objectstore"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

// TrackedObjectStore is a ObjectStore that is also a worker, to ensure the
// lifecycle of the objectStore is managed.
type TrackedObjectStore interface {
	worker.Worker
	objectstore.ObjectStore
}

// Option is the function signature for the options to create a new object
// store.
type Option func(*options)

// WithMongoSession is the option to set the mongo session to use.
func WithMongoSession(session MongoSession) Option {
	return func(o *options) {
		o.mongoSession = session
	}
}

// WithLogger is the option to set the logger to use.
func WithLogger(logger Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

type options struct {
	mongoSession MongoSession
	logger       Logger
}

func newOptions() *options {
	return &options{
		logger: loggo.GetLogger("juju.objectstore"),
	}
}

// ObjectStoreWorkerFunc is the function signature for creating a new object
// store worker.
type ObjectStoreWorkerFunc func(context.Context, BackendType, string, ...Option) (TrackedObjectStore, error)

// BackendType is the type to identify the backend to use for the object store.
type BackendType string

const (
	// StateBackend is the backend type for the state object store.
	StateBackend BackendType = "state"
	// FileBackend is the backend type for the file object store.
	FileBackend BackendType = "file"
)

// ObjectStoreFactory is the function to create a new object store based on
// the backend type.
func ObjectStoreFactory(ctx context.Context, backendType BackendType, namespace string, options ...Option) (TrackedObjectStore, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}
	switch backendType {
	case StateBackend:
		return NewStateObjectStore(ctx, namespace, opts.mongoSession, opts.logger)
	default:
		return nil, errors.NotValidf("backend type %q", backendType)
	}
}

func DefaultBackendType() BackendType {
	return StateBackend
}
