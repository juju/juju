// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"github.com/juju/juju/secrets/provider"
)

const (
	// Store is the name of the Juju secrets store.
	Store = "juju"
)

// NewSecretStore returns a nil store since the Juju store saves
// secret content to the Juju database.
func NewSecretStore(cfg provider.StoreConfig) (provider.SecretsStore, error) {
	return nil, nil
}
