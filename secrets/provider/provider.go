// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/collections/set"
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

// NameMetaSlice stores information for generating resource name for a list of secrets.
type NameMetaSlice map[string]set.Ints

// Add adds a secret with revisions.
func (nm NameMetaSlice) Add(uri *secrets.URI, revisions ...int) {
	if _, ok := nm[uri.ID]; !ok {
		nm[uri.ID] = set.NewInts(revisions...)
		return
	}
	for _, rev := range revisions {
		nm[uri.ID].Add(rev)
	}
}

// Names returns the names of the secrets.
func (nm NameMetaSlice) Names() (names []string) {
	for id, revisions := range nm {
		for _, rev := range revisions.SortedValues() {
			names = append(names, fmt.Sprintf("%s-%d", id, rev))
		}
	}
	return names
}

// SecretStoreProvider instances create secret stores.
type SecretStoreProvider interface {
	// TODO(wallyworld) - add config schema methods

	Type() string

	// Initialise sets up the secrets store to host secrets for
	// the specified model.
	Initialise(m Model) error

	// CleanupSecrets removes any ACLs / resources associated
	// with the removed secrets.
	CleanupSecrets(m Model, tag names.Tag, removed NameMetaSlice) error

	// CleanupModel removes any secrets / ACLs / resources
	// associated with the model.
	CleanupModel(m Model) error

	// StoreConfig returns the config needed to create a vault secrets store client
	// used to manage owned secrets and read shared secrets.
	StoreConfig(m Model, tag names.Tag, owned NameMetaSlice, read NameMetaSlice) (*StoreConfig, error)

	// NewStore creates a secrets store client using the
	// specified config.
	NewStore(cfg *StoreConfig) (SecretsStore, error)
}
