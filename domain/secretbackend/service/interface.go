// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secretbackend"
)

// State provides methods for working with secret backends.
type State interface {
	GetModel(ctx context.Context, uuid coremodel.UUID) (*coremodel.Model, string, error)

	UpsertSecretBackend(ctx context.Context, params secretbackend.UpsertSecretBackendParams) (string, error)
	DeleteSecretBackend(ctx context.Context, backendID string, force bool) error
	ListSecretBackends(ctx context.Context) ([]*coresecrets.SecretBackend, error)
	GetSecretBackendByName(ctx context.Context, name string) (*coresecrets.SecretBackend, error)
	GetSecretBackend(ctx context.Context, backendID string) (*coresecrets.SecretBackend, error)
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error

	GetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, error)
	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error

	InitialWatchStatement() (string, string)
	GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
}
