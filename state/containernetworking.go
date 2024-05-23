// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stdcontext "context"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
)

// AutoConfigureContainerNetworking tries to set up best container networking available
// for the specific model if user hasn't set anything.
func (m *Model) AutoConfigureContainerNetworking(environ environs.BootstrapEnviron, providerConfigSchemaGetter config.ConfigSchemaSourceGetter) error {
	updateAttrs := make(map[string]interface{})
	modelConfig, err := m.ModelConfig(stdcontext.Background())
	if err != nil {
		return err
	}

	if modelConfig.ContainerNetworkingMethod() != "" {
		// Do nothing, user has decided what to do
	} else if environs.SupportsContainerAddresses(envcontext.WithoutCredentialInvalidator(stdcontext.Background()), environ) {
		updateAttrs["container-networking-method"] = "provider"
	} else {
		updateAttrs["container-networking-method"] = "local"
	}
	err = m.UpdateModelConfig(providerConfigSchemaGetter, updateAttrs, nil)
	return err
}
