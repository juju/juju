// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v4"

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

// MetadataService is the interface that is used to get a object store.
type MetadataService interface {
	ObjectStore() objectstore.ObjectStoreMetadata
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

// WithRootDir is the option to set the root directory to use.
func WithRootDir(rootDir string) Option {
	return func(o *options) {
		o.rootDir = rootDir
	}
}

// WithMongoSession is the option to set the mongo session to use.
func WithMongoSession(session MongoSession) Option {
	return func(o *options) {
		o.mongoSession = session
	}
}

// WithS3Client is the option to set the s3 client to use.
func WithS3Client(client objectstore.Client) Option {
	return func(o *options) {
		o.s3Client = client
	}
}

// WithMetadataService is the option to set the metadata service to use.
func WithMetadataService(metadataService MetadataService) Option {
	return func(o *options) {
		o.metadataService = metadataService
	}
}

// WithLogger is the option to set the logger to use.
func WithLogger(logger Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithClaimer is the option to set the claimer to use.
func WithClaimer(claimer Claimer) Option {
	return func(o *options) {
		o.claimer = claimer
	}
}

// WithClock is the option to set the clock to use.
func WithClock(clock clock.Clock) Option {
	return func(o *options) {
		o.clock = clock
	}
}

type options struct {
	rootDir         string
	mongoSession    MongoSession
	s3Client        objectstore.Client
	metadataService MetadataService
	claimer         Claimer
	logger          Logger
	clock           clock.Clock
}

func newOptions() *options {
	return &options{
		logger: loggo.GetLogger("juju.objectstore"),
		clock:  clock.WallClock,
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
	case objectstore.FileBackend:
		return NewFileObjectStore(ctx, namespace, opts.rootDir, opts.metadataService.ObjectStore(), opts.claimer, opts.logger, opts.clock)
	case objectstore.S3Backend:
		return NewS3ObjectStore(ctx, S3ObjectStoreConfig{
			Namespace:       namespace,
			Client:          opts.s3Client,
			MetadataService: opts.metadataService.ObjectStore(),
			Claimer:         opts.claimer,
			Logger:          opts.logger,
			Clock:           opts.clock,
		})
	default:
		return nil, errors.NotValidf("backend type %q", backendType)
	}
}

// BackendTypeOrDefault returns the default backend type for the given object
// store type or falls back to the default backend type.
func BackendTypeOrDefault(objectStoreType objectstore.BackendType) objectstore.BackendType {
	if s, err := objectstore.ParseObjectStoreType(objectStoreType.String()); err == nil {
		return s
	}
	return DefaultBackendType()
}

// DefaultBackendType returns the default backend type for the given object
// store type or falls back to the default backend type.
func DefaultBackendType() objectstore.BackendType {
	return objectstore.StateBackend
}
