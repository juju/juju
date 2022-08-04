// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"time"

	"github.com/juju/juju/core/secrets"
)

const (
	// Version describes the secret format.
	Version = 1
)

// CreateParams are used to create a secret.
type CreateParams struct {
	ProviderLabel  string
	Version        int
	Owner          string
	RotateInterval time.Duration
	Description    string
	Tags           map[string]string
	Params         map[string]interface{}
	Data           map[string]string
}

// UpdateParams are used to update a secret.
type UpdateParams struct {
	RotateInterval *time.Duration
	Description    *string
	Tags           *map[string]string
	Params         map[string]interface{}
	Data           map[string]string
}

// Filter is used when querying secrets.
type Filter struct {
	// TODO(wallyworld)
}

// SecretsService instances provide a backend for storing secrets values.
type SecretsService interface {
	// CreateSecret creates a new secret and returns a URL for accessing the secret value.
	CreateSecret(context.Context, *secrets.URI, CreateParams) (*secrets.SecretMetadata, error)

	// UpdateSecret updates a given secret with a new secret value.
	UpdateSecret(context.Context, *secrets.URI, UpdateParams) (*secrets.SecretMetadata, error)

	// DeleteSecret deletes the specified secret.
	DeleteSecret(context.Context, *secrets.URI) error

	// GetSecret returns the metadata for the specified secret.
	GetSecret(context.Context, *secrets.URI) (*secrets.SecretMetadata, error)

	// GetSecretValue returns the value of the specified secret.
	GetSecretValue(context.Context, *secrets.URI, int) (secrets.SecretValue, error)

	// ListSecrets returns secret metadata using the specified filter.
	ListSecrets(context.Context, Filter) ([]*secrets.SecretMetadata, error)
}

// ProviderConfig is used when constructing a secrets provider.
// TODO(wallyworld) - use a schema
type ProviderConfig map[string]interface{}
