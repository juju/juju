// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

// SecretsBackend is an external secrets backend like vault.
type SecretsBackend interface {
	Ping() error
	SaveContent(_ context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error)
	GetContent(_ context.Context, revisionId string) (secrets.SecretValue, error)

	// DeleteContent removes the specified content.
	// It *must* return a NotFound error if the content does not exist.
	// This is needed so that juju can handle the case where is secret
	// has been drained and added to a new active backend.
	DeleteContent(_ context.Context, revisionId string) error
}

// BackendConfig is used when constructing a secrets backend.
type BackendConfig struct {
	BackendType string
	Config      ConfigAttrs
}

// ModelBackendConfig is used when constructing a secrets backend
// for a particular model.
type ModelBackendConfig struct {
	ControllerUUID string
	ModelUUID      string
	ModelName      string
	BackendConfig
}

// ModelBackendConfigInfo holds secret backends, one of which
// is the active backend for a model.
type ModelBackendConfigInfo struct {
	ActiveID string
	Configs  map[string]ModelBackendConfig
}
