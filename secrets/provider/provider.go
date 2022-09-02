// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
)

// Model exposes the methods needed to create a secrets store config.
type Model interface {
	ControllerUUID() string
	Cloud() (cloud.Cloud, error)
	CloudCredential() (*cloud.Credential, error)
	Config() (*config.Config, error)
}

// SecretStoreProvider instances create secret stores.
type SecretStoreProvider interface {
	// TODO(wallyworld) - add config schema methods

	StoreConfig(m Model) (*StoreConfig, error)
	NewStore(cfg *StoreConfig) (SecretsStore, error)
}
