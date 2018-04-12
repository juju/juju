// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/useragent"
)

// cloudSpecAuth is an implementation of autorest.Authorizer.
type cloudSpecAuth struct {
	cloud  environs.CloudSpec
	sender autorest.Sender
	mu     sync.Mutex
	token  *adal.ServicePrincipalToken
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
			authorizer := autorest.NewBearerAuthorizer(token)
			return autorest.CreatePreparer(authorizer.WithAuthorization()).Prepare(r)
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

func (c *cloudSpecAuth) getToken() (*adal.ServicePrincipalToken, error) {
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
func AuthToken(cloud environs.CloudSpec, sender autorest.Sender) (*adal.ServicePrincipalToken, error) {
	if authType := cloud.Credential.AuthType(); authType != clientCredentialsAuthType {
		// We currently only support a single auth-type for
		// non-interactive authentication. Interactive auth
		// is used only to generate a service-principal.
		return nil, errors.NotSupportedf("auth-type %q", authType)
	}

	resourceId, err := azureauth.ResourceManagerResourceId(cloud.StorageEndpoint)
	if err != nil {
		return nil, errors.Trace(err)
	}

	credAttrs := cloud.Credential.Attributes()
	subscriptionId := credAttrs[credAttrSubscriptionId]
	appId := credAttrs[credAttrAppId]
	appPassword := credAttrs[credAttrAppPassword]
	client := subscriptions.GroupClient{subscriptions.NewWithBaseURI(cloud.Endpoint)}
	useragent.UpdateClient(&client.Client)
	client.Sender = sender
	oauthConfig, _, err := azureauth.OAuthConfig(client, cloud.Endpoint, subscriptionId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	token, err := adal.NewServicePrincipalToken(
		*oauthConfig,
		appId,
		appPassword,
		resourceId,
	)
	if err != nil {
		return nil, errors.Annotate(err, "constructing service principal token")
	}
	tokenClient := autorest.NewClientWithUserAgent("")
	useragent.UpdateClient(&tokenClient)
	tokenClient.Sender = sender
	token.SetSender(&tokenClient)
	return token, nil
}
