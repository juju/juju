// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secretbackend"
)

// State provides methods for working with secret backends.
type State interface {
	GetModel(ctx context.Context, uuid coremodel.UUID) (secretbackend.ModelSecretBackend, error)

	CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error)
	UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) (string, error)
	DeleteSecretBackend(ctx context.Context, backendID string, force bool) error
	ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error)
	GetSecretBackend(context.Context, secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error)
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error

	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, backendName string) error
	GetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (string, error)

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
