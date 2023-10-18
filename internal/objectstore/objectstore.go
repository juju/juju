// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/objectstore/file"
	"github.com/juju/juju/internal/objectstore/state"
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

// MongoSession is the interface that is used to get a mongo session.
// Deprecated: is only here for backwards compatibility.
type MongoSession interface {
	MongoSession() *mgo.Session
}

// Type represents the type of object store.
type Type string

const (
	// StateType represents the state object store.
	// Deprecated: is only here for backwards compatibility.
	StateType Type = "state"
	// FileType represents the file object store.
	FileType Type = "file"
)

// TrackedObjectStore is a ObjectStore that is also a worker, to ensure the
// lifecycle of the objectStore is managed.
type TrackedObjectStore interface {
	objectstore.ObjectStore
	worker.Worker
}

// Config defines the configuration for the object store.
type Config struct {
	MongoSession MongoSession
	RootDir      string
	Logger       Logger
}

// NewObjectStore returns a new object store worker based on the type.
func NewObjectStore(t Type, namespace string, config Config) (TrackedObjectStore, error) {
	switch t {
	case StateType:
		return state.New(namespace, config.MongoSession, config.Logger)
	case FileType:
		return file.New(config.RootDir, namespace, config.Logger)
	default:
		return nil, errors.NotValidf("storage type %t", t)
	}
}
