// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/secret"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/secrets/provider"
)

// State provides methods for working with secret backends.
type State interface {
	// CreateSecretBackend creates a new secret backend and returns its ID.
	CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error)

	// UpdateSecretBackend updates an existing secret backend and returns its ID.
	UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) (string, error)

	// DeleteSecretBackend deletes a secret backend. If deleteInUse is true,
	// the backend is deleted even if it is in use.
	DeleteSecretBackend(ctx context.Context, _ secretbackend.BackendIdentifier, deleteInUse bool) error

	// GetSecretBackend returns the secret backend identified by the given
	// identifier (name or ID).
	GetSecretBackend(context.Context, secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error)

	// ListSecretBackends returns all secret backends.
	ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error)

	// ListSecretBackendIDs returns the IDs of all secret backends.
	ListSecretBackendIDs(ctx context.Context) ([]string, error)

	// SecretBackendRotated records that a backend token was rotated and
	// schedules the next rotation at the given time.
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error

	// SetModelSecretBackend sets the secret backend for a model.
	SetModelSecretBackend(ctx context.Context, modelUUID coremodel.UUID, secretBackendName string) error

	// ListSecretBackendsForModel returns the secret backends configured for
	// the given model. If includeEmpty is true, the built-in backend is
	// included even if no model override is set.
	ListSecretBackendsForModel(ctx context.Context, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error)

	// GetModelSecretBackendDetails returns the secret backend details for
	// the given model.
	GetModelSecretBackendDetails(ctx context.Context, modelUUID coremodel.UUID) (secretbackend.ModelSecretBackend, error)

	// GetModelType returns the model type for the given model UUID.
	GetModelType(ctx context.Context, modelUUID coremodel.UUID) (coremodel.ModelType, error)

	// GetInternalAndActiveBackendUUIDs returns the UUIDs of the internal
	// and active secret backends for the given model.
	GetInternalAndActiveBackendUUIDs(ctx context.Context, modelUUID coremodel.UUID) (string, string, error)

	// InitialWatchStatementForSecretBackendRotationChanges returns the
	// table name and initial watch statement for secret backend rotation
	// changes.
	InitialWatchStatementForSecretBackendRotationChanges() (string, string)

	// GetSecretBackendRotateChanges returns the rotation change events for
	// the given backend IDs.
	GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)

	// NamespaceForWatchModelSecretBackend returns the namespace used to
	// watch for model secret backend changes.
	NamespaceForWatchModelSecretBackend() string

	// AddSecretBackendReference adds a reference to track that a secret
	// revision is stored in the specified backend and returns a rollback
	// function to remove the reference if needed.
	AddSecretBackendReference(ctx context.Context, valueRef *secrets.ValueRef, modelID coremodel.UUID, revisionID string, secretID string) (func() error, error)
}

// AdminBackendConfigGetterFunc returns a function that gets the
// admin config for a given model's current secret backend.
func AdminBackendConfigGetterFunc(
	backendService *WatchableService, modelUUID coremodel.UUID,
) func(stdCtx context.Context) (*provider.ModelBackendConfigInfo, error) {
	return func(stdCtx context.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(stdCtx, modelUUID)
	}
}

// UserSecretBackendConfigGetterFunc returns a function that gets the
// config for a given model's current secret backend for creating or updating user secrets.
func UserSecretBackendConfigGetterFunc(backendService *WatchableService, modelUUID coremodel.UUID) func(
	stdCtx context.Context, gsg secretservice.GrantedSecretsGetter, accessor secret.SecretAccessor,
) (*provider.ModelBackendConfigInfo, error) {
	return func(
		stdCtx context.Context, gsg secretservice.GrantedSecretsGetter, accessor secret.SecretAccessor,
	) (*provider.ModelBackendConfigInfo, error) {
		return backendService.BackendConfigInfo(stdCtx, BackendConfigParams{
			GrantedSecretsGetter: gsg,
			Accessor:             accessor,
			ModelUUID:            modelUUID,
			SameController:       true,
		})
	}
}
