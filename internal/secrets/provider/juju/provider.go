// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"context"

	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/internal/secrets/provider"
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
func (p jujuProvider) CleanupSecrets(_ context.Context, _ *provider.ModelBackendConfig, _ *secrets.URI, _ provider.SecretRevisions) error {
	return nil
}

// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p jujuProvider) RestrictedConfig(
	_ context.Context, _ *provider.ModelBackendConfig, _, _ bool, _ secrets.Accessor, _ provider.SecretRevisions, _ provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	return &provider.BackendConfig{
		BackendType: BackendType,
	}, nil
}

// NewBackend returns a nil backend since the Juju backend saves
// secret content to the Juju database.
func (jujuProvider) NewBackend(_ *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
	return &jujuBackend{}, nil
}
