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
type ObjectStoreWorkerFunc func(context.Context, objectstore.BackendType, string, ...Option) (TrackedObjectStore, error)

// ObjectStoreFactory is the function to create a new object store based on
// the backend type.
func ObjectStoreFactory(ctx context.Context, backendType objectstore.BackendType, namespace string, options ...Option) (TrackedObjectStore, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}
	switch backendType {
	case objectstore.StateBackend:
		return NewStateObjectStore(ctx, namespace, opts.mongoSession, opts.logger)
	default:
		return nil, errors.NotValidf("backend type %q", backendType)
	}
}

func DefaultBackendType() objectstore.BackendType {
	return objectstore.StateBackend
}
