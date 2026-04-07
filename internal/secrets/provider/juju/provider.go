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
	// UnknownBackendName is the name of an unknown secrets backend.
	UnknownBackendName = "<unknown>"
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

// Initialise is not used because this provider does not have any external
// interactions outside the model.
func (p jujuProvider) Initialise(*provider.ModelBackendConfig) error {
	return nil
}

<<<<<<< HEAD:internal/secrets/provider/juju/provider.go
// CleanupModel is not used.
func (p jujuProvider) CleanupModel(context.Context, *provider.ModelBackendConfig) error {
	return nil
}

// CleanupSecrets is not used.
func (p jujuProvider) CleanupSecrets(_ context.Context, _ *provider.ModelBackendConfig, _ secrets.Accessor, _ provider.SecretRevisions) error {
	return nil
}

=======
// CleanupModel is not used because this provider does not have any resources
// that exist outside of the model.
func (p jujuProvider) CleanupModel(*provider.ModelBackendConfig) error {
	return nil
}

// CleanupSecrets is not used because this provider does not store secrets
// externally to the model.
func (p jujuProvider) CleanupSecrets(cfg *provider.ModelBackendConfig, tag names.Tag, removed provider.SecretRevisions) error {
	return nil
}

// CleanupIssuedTokens is not used because this provider does not issue backend
// tokens.
func (p jujuProvider) CleanupIssuedTokens(
	_ *provider.ModelBackendConfig, issuedTokenUUIDs []string,
) ([]string, error) {
	return issuedTokenUUIDs, nil
}

// BuiltInConfig returns a minimal config for the Juju backend.
func BuiltInConfig() provider.BackendConfig {
	return provider.BackendConfig{BackendType: BackendType}
}

// IssuesTokens returns false since this provider does not create tokens.
func (p jujuProvider) IssuesTokens() bool {
	return false
}

>>>>>>> 3.6:secrets/provider/juju/provider.go
// RestrictedConfig returns the config needed to create a
// secrets backend client restricted to manage the specified
// owned secrets and read shared secrets for the given entity tag.
func (p jujuProvider) RestrictedConfig(
<<<<<<< HEAD:internal/secrets/provider/juju/provider.go
	_ context.Context, _ *provider.ModelBackendConfig, _, _ bool, _ secrets.Accessor, _ provider.SecretRevisions, _ provider.SecretRevisions,
=======
	*provider.ModelBackendConfig,
	bool, bool, string, names.Tag,
	[]string, provider.SecretRevisions, provider.SecretRevisions,
>>>>>>> 3.6:secrets/provider/juju/provider.go
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
