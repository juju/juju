// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/secrets/provider"
)

const (
	// Backend is the name of the Juju secrets backend.
	Backend = "juju"
)

// NewProvider returns a Juju secrets provider.
func NewProvider() provider.SecretBackendProvider {
	return jujuProvider{Backend}
}

type jujuProvider struct {
	name string
}

func (p jujuProvider) Type() string {
	return p.name
}

// Initialise is not used.
func (p jujuProvider) Initialise(m provider.Model) error {
	return nil
}

// CleanupModel is not used.
func (p jujuProvider) CleanupModel(m provider.Model) error {
	return nil
}

// CleanupSecrets is not used.
func (p jujuProvider) CleanupSecrets(m provider.Model, tag names.Tag, removed provider.SecretRevisions) error {
	return nil
}

// BackendConfig returns nil config params since the Juju backend saves
// secret content to the Juju database.
func (p jujuProvider) BackendConfig(
	m provider.Model, tag names.Tag, owned provider.SecretRevisions, read provider.SecretRevisions,
) (*provider.BackendConfig, error) {
	return &provider.BackendConfig{BackendType: Backend}, nil
}

// NewBackend returns a nil backend since the Juju backend saves
// secret content to the Juju database.
func (jujuProvider) NewBackend(*provider.BackendConfig) (provider.SecretsBackend, error) {
	return nil, nil
}
