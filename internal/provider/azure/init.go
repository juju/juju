// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/utils/v4/ssh"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/azure/internal/azureauth"
	"github.com/juju/juju/internal/provider/azure/internal/azurecli"
)

const (
	// providerType is the unique identifier that the azure provider gets
	// registered with.
	providerType = "azure"
)

// NewProvider instantiates and returns the Azure EnvironProvider using the
// given configuration.
func NewProvider(config ProviderConfig) (environs.CloudEnvironProvider, error) {
	environProvider, err := NewEnvironProvider(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return environProvider, nil
}

func init() {
	environProvider, err := NewProvider(ProviderConfig{
		RetryClock:              &clock.WallClock,
		GenerateSSHKey:          ssh.GenerateKey,
		ServicePrincipalCreator: &azureauth.ServicePrincipalCreator{},
		AzureCLI:                azurecli.AzureCLI{},
		CreateTokenCredential: func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error) {
			return azidentity.NewClientSecretCredential(
				tenantID, appId, appPassword, &azidentity.ClientSecretCredentialOptions{
					ClientOptions: opts,
				},
			)
		},
	})
	if err != nil {
		panic(err)
	}

	environs.RegisterProvider(providerType, environProvider)

	// TODO(axw) register an image metadata data source that queries
	// the Azure image registry, and introduce a way to disable the
	// common simplestreams source.
}
