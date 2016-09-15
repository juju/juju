// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrAppId          = "application-id"
	credAttrSubscriptionId = "subscription-id"
	credAttrAppPassword    = "application-password"
)

// environPoviderCredentials is an implementation of
// environs.ProviderCredentials for the Azure Resource
// Manager cloud provider.
type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		cloud.UserPassAuthType: {
			{
				credAttrAppId, cloud.CredentialAttr{Description: "Azure Active Directory application ID"},
			}, {
				credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
			}, {
				credAttrAppPassword, cloud.CredentialAttr{
					Description: "Azure Active Directory application password",
					Hidden:      true,
				},
			},
		},
	}
}

// DetectCredentials is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) DetectCredentials() (*cloud.CloudCredential, error) {
	return nil, errors.NotFoundf("credentials")
}

// FinalizeCredential is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) FinalizeCredential(ctx environs.FinalizeCredentialContext, in cloud.Credential) (cloud.Credential, error) {
	return in, nil
}
