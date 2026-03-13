// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/domain/secretbackend/internal"
)

// State provides methods for working with secret backends.
type State interface {
	CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error)
	UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) (string, error)
	DeleteSecretBackend(ctx context.Context, _ secretbackend.BackendIdentifier, deleteInUse bool) error
	GetSecretBackend(context.Context, secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error)
	ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error)
	ListSecretBackendIDs(ctx context.Context) ([]string, error)
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error
	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, secretBackendName string) error

	ListSecretBackendsForModel(ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error)
	GetModelSecretBackendNameAndOrigin(ctx context.Context, modelUUID coremodel.UUID) (string, internal.Origin, error)
	GetModelSecretBackendDetails(ctx context.Context, modelUUID coremodel.UUID) (secretbackend.ModelSecretBackend, error)
	GetModelType(ctx context.Context, modelUUID coremodel.UUID) (coremodel.ModelType, error)

	GetInternalAndActiveBackendUUIDs(ctx context.Context, modelUUID coremodel.UUID) (string, string, error)

	InitialWatchStatementForSecretBackendRotationChanges() (string, string)
	GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)
	NamespaceForWatchModelSecretBackend() string
}
