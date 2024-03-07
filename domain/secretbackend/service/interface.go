// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
)

// State provides methods for working with secret backends.
type State interface {
	CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error)
	UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) error
	DeleteSecretBackend(ctx context.Context, backendID string, force bool) error
	ListSecretBackends(ctx context.Context) ([]*coresecrets.SecretBackend, error)
	GetSecretBackendByName(ctx context.Context, name string) (*coresecrets.SecretBackend, error)
	GetSecretBackend(ctx context.Context, backendID string) (*coresecrets.SecretBackend, error)
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error

	IncreCountForSecretBackend(ctx context.Context, backendID string) error
	DecreCountForSecretBackend(ctx context.Context, backendID string) error

	WatchSecretBackendRotationChanges(context.Context, secretbackend.WatcherFactory) (secretbackend.SecretBackendRotateWatcher, error)
}

// Logger facilitates emitting log messages.
type Logger interface {
	Debugf(string, ...interface{})
}
