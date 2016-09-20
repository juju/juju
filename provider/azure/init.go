// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/azurestorage"
)

const (
	providerType = "azure"
)

// NewProvider instantiates and returns the Azure EnvironProvider using the
// given configuration.
func NewProvider(config ProviderConfig) (environs.EnvironProvider, error) {
	environProvider, err := NewEnvironProvider(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return environProvider, nil
}

func init() {
	environProvider, err := NewProvider(ProviderConfig{
		NewStorageClient:                  azurestorage.NewClient,
		RetryClock:                        &clock.WallClock,
		RandomWindowsAdminPassword:        randomAdminPassword,
		InteractiveCreateServicePrincipal: azureauth.InteractiveCreateServicePrincipal,
	})
	if err != nil {
		panic(err)
	}

	environs.RegisterProvider(providerType, environProvider)

	// TODO(axw) register an image metadata data source that queries
	// the Azure image registry, and introduce a way to disable the
	// common simplestreams source.
}
