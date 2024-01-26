// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/names/v5"

	"github.com/juju/juju/secrets/provider"
)

const (
	// BackendName is the name of the Juju secrets backend.
	BackendName = "internal"
	// BackendType is the type of the Juju secrets backend.
	BackendType = "controller"
)

// NewProvider returns a Juju secrets provider.
func NewProvider() provider.SecretBackendProvider {
	return jujuProvider{}
}

type jujuProvider struct {
}

func (p jujuProvider) Type() string {
	return BackendType
}

// Initialise is not used.
func (p jujuProvider) Initialise(*provider.ModelBackendConfig) error {
	return nil
}

// CleanupModel is not used.
func (p jujuProvider) CleanupModel(*provider.ModelBackendConfig) error {
	return nil
}

// CleanupSecrets is not used.
func (p jujuProvider) CleanupSecrets(cfg *provider.ModelBackendConfig, tag names.Tag, removed provider.SecretRevisions) error {
	return nil
}

// BuiltInConfig returns a minimal config for the Juju backend.
func BuiltInConfig() provider.BackendConfig {
	return provider.BackendConfig{BackendType: BackendType}
}

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p jujuProvider) RestrictedConfig(
	adminCfg *provider.ModelBackendConfig, sameController, forDrain bool, tag names.Tag, owned provider.SecretRevisions, read provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	return &provider.BackendConfig{
		BackendType: BackendType,
	}, nil
}

// NewBackend returns a nil backend since the Juju backend saves
// secret content to the Juju database.
func (jujuProvider) NewBackend(*provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	return &jujuBackend{}, nil
}
