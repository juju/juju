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
	GetContent(_ context.Context, backendId string) (secrets.SecretValue, error)
	DeleteContent(_ context.Context, backendId string) error
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

// ModelBackendConfigInfo holds all the secret backends relevant
// for a particular model.
type ModelBackendConfigInfo struct {
	ControllerUUID string
	ModelUUID      string
	ModelName      string
	ActiveID       string
	Configs        map[string]BackendConfig
}
