// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

const (
	// Version describes the secret format.
	Version = 1
)

// CreateParams are used to create a secret.
type CreateParams struct {
	ControllerUUID string
	ModelUUID      string
	ProviderLabel  string
	Version        int
	Type           string
	Path           string
	Scope          string
	Params         map[string]interface{}
	Data           map[string]string
}

// UpdateParams are used to update a secret.
type UpdateParams struct {
	Params map[string]interface{}
	Data   map[string]string
}

// Filter is used when querying secrets.
type Filter struct {
	// TODO(wallyworld)
}

// SecretsService instances provide a backend for storing secrets values.
type SecretsService interface {
	// CreateSecret creates a new secret and returns a URL for accessing the secret value.
	CreateSecret(ctx context.Context, p CreateParams) (*secrets.URL, *secrets.SecretMetadata, error)

	// UpdateSecret updates a given secret with a new secret value.
	UpdateSecret(ctx context.Context, URL *secrets.URL, p UpdateParams) (*secrets.SecretMetadata, error)

	// DeleteSecret deletes the specified secret.
	DeleteSecret(ctx context.Context, URL *secrets.URL) error

	// GetSecret returns the metadata for the specified secret.
	GetSecret(ctx context.Context, URL *secrets.URL) (*secrets.SecretMetadata, error)

	// GetSecretValue returns the value of the specified secret.
	GetSecretValue(ctx context.Context, URL *secrets.URL) (secrets.SecretValue, error)

	// ListSecrets returns secret metadata using the specified filter.
	ListSecrets(ctx context.Context, filter Filter) ([]*secrets.SecretMetadata, error)
}

// ProviderConfig is used when constructing a secrets provider.
// TODO(wallyworld) - use a schema
type ProviderConfig map[string]interface{}
