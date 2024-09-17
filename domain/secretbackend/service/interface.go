// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/secrets/provider"
)

// AtomicState describes retrieval and persistence methods for
// secret backends that require atomic transactions.
type AtomicState interface {
	domain.AtomicStateBase

	GetModelSecretBackendDetails(ctx domain.AtomicContext, modelUUID coremodel.UUID) (secretbackend.ModelSecretBackend, error)
	GetSecretBackend(domain.AtomicContext, secretbackend.BackendIdentifier) (*secretbackend.SecretBackend, error)
	ListSecretBackendsForModel(ctx domain.AtomicContext, modelUUID coremodel.UUID, includeEmpty bool) ([]*secretbackend.SecretBackend, error)

	SetModelSecretBackend(ctx domain.AtomicContext, modelUUID coremodel.UUID, secretBackendName string) error
}

// State provides methods for working with secret backends.
type State interface {
	AtomicState

	CreateSecretBackend(ctx context.Context, params secretbackend.CreateSecretBackendParams) (string, error)
	UpdateSecretBackend(ctx context.Context, params secretbackend.UpdateSecretBackendParams) (string, error)
	DeleteSecretBackend(ctx context.Context, _ secretbackend.BackendIdentifier, deleteInUse bool) error
	ListSecretBackends(ctx context.Context) ([]*secretbackend.SecretBackend, error)
	ListSecretBackendIDs(ctx context.Context) ([]string, error)
	SecretBackendRotated(ctx context.Context, backendID string, next time.Time) error

	InitialWatchStatementForSecretBackendRotationChanges() (string, string)
	GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, eventsource.NamespaceQuery) (watcher.StringsWatcher, error)

	// NewValueWatcher returns a watcher for a particular change
	// value in a namespace, based on the input change mask.
	NewValueWatcher(namespace, changeValue string, changeMask changestream.ChangeType) (watcher.NotifyWatcher, error)
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
	stdCtx context.Context, gsg secretservice.GrantedSecretsGetter, accessor secretservice.SecretAccessor,
) (*provider.ModelBackendConfigInfo, error) {
	return func(
		stdCtx context.Context, gsg secretservice.GrantedSecretsGetter, accessor secretservice.SecretAccessor,
	) (*provider.ModelBackendConfigInfo, error) {
		return backendService.BackendConfigInfo(stdCtx, BackendConfigParams{
			GrantedSecretsGetter: gsg,
			Accessor:             accessor,
			ModelUUID:            modelUUID,
			SameController:       true,
		})
	}
}
