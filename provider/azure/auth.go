// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"net/http"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azureauth"
)

// cloudSpecAuth is an implementation of autorest.Authorizer.
type cloudSpecAuth struct {
	cloud  environs.CloudSpec
	sender autorest.Sender
	mu     sync.Mutex
	token  *azure.ServicePrincipalToken
}

// WithAuthorization is part of the autorest.Authorizer interface.
func (c *cloudSpecAuth) WithAuthorization() autorest.PrepareDecorator {
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(r *http.Request) (*http.Request, error) {
			r, err := p.Prepare(r)
			if err != nil {
				return nil, err
			}
			token, err := c.getToken()
			if err != nil {
				return nil, err
			}
			return autorest.CreatePreparer(token.WithAuthorization()).Prepare(r)
		})
	}
}

func (c *cloudSpecAuth) refresh() error {
	token, err := c.getToken()
	if err != nil {
		return err
	}
	return token.Refresh()
}

func (c *cloudSpecAuth) getToken() (*azure.ServicePrincipalToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != nil {
		return c.token, nil
	}
	token, err := AuthToken(c.cloud, c.sender)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c.token = token
	return c.token, nil
}

// AuthToken returns a service principal token, suitable for authorizing
// Resource Manager API requests, based on the supplied CloudSpec.
func AuthToken(cloud environs.CloudSpec, sender autorest.Sender) (*azure.ServicePrincipalToken, error) {
	credAttrs := cloud.Credential.Attributes()
	subscriptionId := credAttrs[credAttrSubscriptionId]
	appId := credAttrs[credAttrAppId]
	appPassword := credAttrs[credAttrAppPassword]

	subscriptionsClient := subscriptions.Client{
		subscriptions.NewWithBaseURI(cloud.Endpoint),
	}
	if sender != nil {
		subscriptionsClient.Sender = sender
	}
	authURI, err := azureauth.DiscoverAuthorizationURI(subscriptionsClient, subscriptionId)
	if err != nil {
		return nil, errors.Annotate(err, "detecting auth URI")
	}
	logger.Debugf("discovered auth URI: %s", authURI)

	// The authorization URI scheme and host identifies the AD endpoint.
	// The authorization URI path identifies the AD tenant.
	tenantId, err := azureauth.AuthorizationURITenantID(authURI)
	if err != nil {
		return nil, errors.Annotate(err, "getting tenant ID")
	}
	authURI.Path = ""
	adEndpoint := authURI.String()

	cloudEnv := azure.Environment{ActiveDirectoryEndpoint: adEndpoint}
	oauthConfig, err := cloudEnv.OAuthConfigForTenant(tenantId)
	if err != nil {
		return nil, errors.Annotate(err, "getting OAuth configuration")
	}

	// We want to create a service principal token for the resource
	// manager endpoint. Azure demands that the URL end with a '/'.
	resource := cloud.Endpoint
	if !strings.HasSuffix(resource, "/") {
		resource += "/"
	}

	token, err := azure.NewServicePrincipalToken(
		*oauthConfig,
		appId,
		appPassword,
		resource,
	)
	if err != nil {
		return nil, errors.Annotate(err, "constructing service principal token")
	}
	if sender != nil {
		token.SetSender(sender)
	}
	return token, nil
}
