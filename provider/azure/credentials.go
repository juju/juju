// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
)

const (
	credAttrAppId          = "application-id"
	credAttrSubscriptionId = "subscription-id"
	credAttrTenantId       = "tenant-id"
	credAttrAppPassword    = "application-password"

	// clientCredentialsAuthType is the auth-type for the
	// "client credentials" OAuth flow, which requires a
	// service principal with a password.
	clientCredentialsAuthType cloud.AuthType = "service-principal-secret"

	// deviceCodeAuthType is the auth-type for the interactive
	// "device code" OAuth flow.
	deviceCodeAuthType cloud.AuthType = "interactive"
)

// environPoviderCredentials is an implementation of
// environs.ProviderCredentials for the Azure Resource
// Manager cloud provider.
type environProviderCredentials struct{}

// CredentialSchemas is part of the environs.ProviderCredentials interface.
func (environProviderCredentials) CredentialSchemas() map[cloud.AuthType]cloud.CredentialSchema {
	return map[cloud.AuthType]cloud.CredentialSchema{
		// TODO(axw) 2016-09-15 #1623761
		// UserPassAuthType is here for backwards
		// compatibility. Drop it when rc1 is out.
		cloud.UserPassAuthType: {
			{
				credAttrAppId, cloud.CredentialAttr{Description: "Azure Active Directory application ID"},
			}, {
				credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
			}, {
				credAttrTenantId, cloud.CredentialAttr{
					Description: "Azure Active Directory tenant ID",
					Optional:    true,
				},
			}, {
				credAttrAppPassword, cloud.CredentialAttr{
					Description: "Azure Active Directory application password",
					Hidden:      true,
				},
			},
		},

		// deviceCodeAuthType is the interactive device-code oauth
		// flow. This is only supported on the client side; it will
		// be used to generate a service principal, and transformed
		// into clientCredentialsAuthType.
		deviceCodeAuthType: {{
			credAttrSubscriptionId, cloud.CredentialAttr{Description: "Azure subscription ID"},
		}},

		// clientCredentialsAuthType is the "client credentials"
		// oauth flow, which requires a service principal with a
		// password.
		clientCredentialsAuthType: {
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
	switch authType := in.AuthType(); authType {
	case cloud.UserPassAuthType:
		fmt.Fprintf(ctx.GetStderr(), `
WARNING: The %q auth-type is deprecated, and will be removed soon.

Please update the credential in ~/.local/share/juju/credentials.yaml,
changing auth-type to %q, and dropping the tenant-id field.

`[1:],
			authType, clientCredentialsAuthType,
		)
		attrs := in.Attributes()
		delete(attrs, credAttrTenantId)
		label := in.Label
		in = cloud.NewCredential(clientCredentialsAuthType, attrs)
		in.Label = label
		return in, nil
	case clientCredentialsAuthType:
		return in, nil
	default:
		return cloud.Credential{}, errors.NotSupportedf("%q auth-type", authType)
	}
}
