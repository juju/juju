// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
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
				configAttrAppId, cloud.CredentialAttr{Description: "Azure Active Directory application ID"},
			}, {
				configAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
			}, {
				configAttrTenantId, cloud.CredentialAttr{Description: "Azure Active Directory tenant ID"},
			}, {
				configAttrAppPassword, cloud.CredentialAttr{
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
