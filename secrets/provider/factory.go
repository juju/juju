// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v3/secrets"
	"github.com/juju/juju/v3/secrets/provider/juju"
)

// NewSecretProvider returns a new secrets provider for the given type.
func NewSecretProvider(providerType string, cfg secrets.ProviderConfig) (secrets.SecretsService, error) {
	switch providerType {
	case juju.Provider:
		return juju.NewSecretService(cfg)
	}
	return nil, errors.NotSupportedf("secrets provider type %q", providerType)
}
