// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	secretservice "github.com/juju/juju/domain/secret/service"
	"github.com/juju/juju/domain/secretbackend"
	"github.com/juju/juju/internal/secrets/provider"
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
	GetModelSecretBackendDetails(ctx context.Context, modelUUID coremodel.UUID) (secretbackend.ModelSecretBackend, error)
	GetModelType(ctx context.Context, modelUUID coremodel.UUID) (coremodel.ModelType, error)

	GetInternalAndActiveBackendUUIDs(ctx context.Context, modelUUID coremodel.UUID) (string, string, error)

	InitialWatchStatementForSecretBackendRotationChanges() (string, string)
	GetSecretBackendRotateChanges(ctx context.Context, backendIDs ...string) ([]watcher.SecretBackendRotateChange, error)
	NamespaceForWatchModelSecretBackend() string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. Change-log events will be emitted only if
	// the filter accepts them, and dispatching the notifications via the
	// Changes channel. A filter option is required, though additional filter
	// options can be provided.
	NewNamespaceWatcher(
		ctx context.Context,
		initialQuery eventsource.NamespaceQuery,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the input
	// base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		ctx context.Context,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
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
