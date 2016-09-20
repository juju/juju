// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/juju/errors"
)

// OAuthConfig returns an azure.OAuthConfig based on the given resource
// manager endpoint and subscription ID. This will make a request to the
// resource manager API to discover the Active Directory tenant ID.
func OAuthConfig(
	client subscriptions.Client,
	resourceManagerEndpoint string,
	subscriptionId string,
) (*azure.OAuthConfig, string, error) {
	authURI, err := DiscoverAuthorizationURI(client, subscriptionId)
	if err != nil {
		return nil, "", errors.Annotate(err, "detecting auth URI")
	}
	logger.Debugf("discovered auth URI: %s", authURI)

	// The authorization URI scheme and host identifies the AD endpoint.
	// The authorization URI path identifies the AD tenant.
	tenantId, err := AuthorizationURITenantID(authURI)
	if err != nil {
		return nil, "", errors.Annotate(err, "getting tenant ID")
	}
	authURI.Path = ""
	adEndpoint := authURI.String()

	cloudEnv := azure.Environment{ActiveDirectoryEndpoint: adEndpoint}
	oauthConfig, err := cloudEnv.OAuthConfigForTenant(tenantId)
	if err != nil {
		return nil, "", errors.Annotate(err, "getting OAuth configuration")
	}
	return oauthConfig, tenantId, nil
}
