// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"context"
	"time"

	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/model"
)

// CreateSecretBackendParams are used to create a secret backend.
type CreateSecretBackendParams struct {
	ID                  string
	Name                string
	BackendType         string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]interface{}
}

// UpdateSecretBackendParams are used to update a secret backend.
type UpdateSecretBackendParams struct {
	ID                  string
	NameChange          *string
	TokenRotateInterval *time.Duration
	NextRotateTime      *time.Time
	Config              map[string]interface{}
}

// SecretBackendFilter is used when listing secret backends.
type SecretBackendFilter struct {
	Names []string
	All   bool
}

// SecretBackendInfo contains information about a secret backend.
type SecretBackendInfo struct {
	secrets.SecretBackend

	NumSecrets int
	Status     string
	Message    string
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
}

// SecretBackendRotateWatcher represents a watcher that returns a slice of SecretBackendRotateChange.
type SecretBackendRotateWatcher interface {
	worker.Worker
	Changes() <-chan []watcher.SecretBackendRotateChange
}

// ModelGetter provides methods for working with models for backend service.
type ModelGetter interface {
	GetModel(ctx context.Context, uuid coremodel.UUID) (*coremodel.Model, error)
	GetSecretBackend(ctx context.Context, modelUUID coremodel.UUID) (model.SecretBackendIdentifier, error)
}
