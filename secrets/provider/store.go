// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"

	"github.com/juju/juju/core/secrets"
)

// SecretsStore is an external secrets store like vault.
type SecretsStore interface {
	GetContent(_ context.Context, providerId string, revision int) (secrets.SecretValue, error)
	SaveContent(_ context.Context, revision int, value secrets.SecretValue) (string, error)
}

// StoreConfig is used when constructing a secrets store.
// TODO(wallyworld) - use a schema
type StoreConfig map[string]interface{}
