// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/environs/config"
)

// Model exposes the methods needed to create a secrets store config.
type Model interface {
	ControllerUUID() string
	Cloud() (cloud.Cloud, error)
	CloudCredential() (*cloud.Credential, error)
	Config() (*config.Config, error)
	UUID() string
}

// SecretStoreProvider instances create secret stores.
type SecretStoreProvider interface {
	// TODO(wallyworld) - add config schema methods

	// Initialise sets up the secrets store to host secrets for
	// the specified model.
	Initialise(m Model) error

	// CleanupSecrets removes any ACLs / resources associated
	// with the removed secrets.
	CleanupSecrets(m Model, tag names.Tag, removed []*secrets.URI) error

	// CleanupModel removes any secrets / ACLs / resources
	// associated with the model.
	CleanupModel(m Model) error

	// StoreConfig returns the config needed to create a vault secrets store client
	// used to manage owned secrets and read shared secrets.
	StoreConfig(m Model, tag names.Tag, owned []*secrets.URI, read []*secrets.URI) (*StoreConfig, error)

	// NewStore creates a secrets store client using the
	// specified config.
	NewStore(cfg *StoreConfig) (SecretsStore, error)
}
