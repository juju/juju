// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"time"

	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
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

// ModelMetadata represents the metadata for a model.
type ModelMetadata struct {
	UUID string
	Name string
	Type string
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
