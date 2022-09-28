// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

// SecretsStore is an external secrets store like vault.
type SecretsStore interface {
	SaveContent(_ context.Context, uri *secrets.URI, revision int, value secrets.SecretValue) (string, error)
	GetContent(_ context.Context, providerId string) (secrets.SecretValue, error)
	DeleteContent(_ context.Context, providerId string) error
}

// StoreConfig is used when constructing a secrets store.
type StoreConfig struct {
	StoreType string
	// TODO(wallyworld) - use a schema
	Params map[string]interface{}
}
