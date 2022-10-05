// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/names/v4"

	"github.com/juju/juju/secrets/provider"
)

const (
	// Store is the name of the Juju secrets store.
	Store = "juju"
)

// NewProvider returns a Juju secrets provider.
func NewProvider() provider.SecretStoreProvider {
	return jujuProvider{Store}
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
func (p jujuProvider) CleanupSecrets(m provider.Model, tag names.Tag, removed provider.NameMetaSlice) error {
	return nil
}

// StoreConfig returns nil config params since the Juju store saves
// secret content to the Juju database.
func (p jujuProvider) StoreConfig(
	m provider.Model, tag names.Tag, owned provider.NameMetaSlice, read provider.NameMetaSlice,
) (*provider.StoreConfig, error) {
	return &provider.StoreConfig{StoreType: Store}, nil
}

// NewStore returns a nil store since the Juju store saves
// secret content to the Juju database.
func (jujuProvider) NewStore(*provider.StoreConfig) (provider.SecretsStore, error) {
	return nil, nil
}
