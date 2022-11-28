// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

// SecretsBackend is an external secrets backend like vault.
type SecretsBackend interface {
	SaveContent(_ context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error)
	GetContent(_ context.Context, backendId string) (secrets.SecretValue, error)
	DeleteContent(_ context.Context, backendId string) error
}

// BackendConfig is used when constructing a secrets backend.
type BackendConfig struct {
	BackendType string
	// TODO(wallyworld) - use a schema
	Config map[string]interface{}
}
