// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"sync"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2016-06-01/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/errors"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/useragent"
)

// cloudSpecAuth provides an implementation of autorest.Authorizer.
type cloudSpecAuth struct {
	cloud    environscloudspec.CloudSpec
	sender   autorest.Sender
	mu       sync.Mutex
	tokens   map[string]*adal.ServicePrincipalToken
	tenantID string
}

func (c *cloudSpecAuth) auth() *autorest.BearerAuthorizerCallback {
	return autorest.NewBearerAuthorizerCallback(c.sender, func(tenantID, resourceID string) (*autorest.BearerAuthorizer, error) {
		token, err := c.getToken(resourceID)
		if err != nil {
			return nil, errors.Annotate(err, "constructing service principal token for secret")
		}
		return autorest.NewBearerAuthorizer(token), nil
	})
}

func (c *cloudSpecAuth) refresh() error {
	token, err := c.getToken("")
	if err != nil {
		return err
	}
	return token.Refresh()
}

func (c *cloudSpecAuth) getToken(resourceID string) (*adal.ServicePrincipalToken, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = make(map[string]*adal.ServicePrincipalToken)
	}

	if resourceID == "" {
		var err error
		resourceID, err = azureauth.ResourceManagerResourceId(c.cloud.StorageEndpoint)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	if token := c.tokens[resourceID]; token != nil {
		return token, nil
	}
	logger.Debugf("get auth token for %v", resourceID)
	token, tenantID, err := AuthToken(c.cloud, c.sender, resourceID)
	if err != nil {
		return nil, errors.Annotate(err, "constructing service principal token for secret")
	}

	c.tokens[resourceID] = token
	c.tenantID = tenantID
	return token, nil
}

// AuthToken returns a service principal token, suitable for authorizing
// Resource Manager API requests, based on the supplied CloudSpec.
func AuthToken(cloud environscloudspec.CloudSpec, sender autorest.Sender, resourceID string) (*adal.ServicePrincipalToken, string, error) {
	if authType := cloud.Credential.AuthType(); authType != clientCredentialsAuthType {
		// We currently only support a single auth-type for
		// non-interactive authentication. Interactive auth
		// is used only to generate a service-principal.
		return nil, "", errors.NotSupportedf("auth-type %q", authType)
	}

	credAttrs := cloud.Credential.Attributes()
	subscriptionId := credAttrs[credAttrSubscriptionId]
	appId := credAttrs[credAttrAppId]
	appPassword := credAttrs[credAttrAppPassword]
	client := subscriptions.Client{subscriptions.NewWithBaseURI(cloud.Endpoint)}
	useragent.UpdateClient(&client.Client)
	client.Sender = sender
	sdkCtx := context.Background()
	oauthConfig, tenantID, err := azureauth.OAuthConfig(sdkCtx, client, subscriptionId)
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	token, err := adal.NewServicePrincipalToken(
		*oauthConfig,
		appId,
		appPassword,
		resourceID,
	)
	if err != nil {
		return nil, "", errors.Annotate(err, "constructing service principal token")
	}
	tokenClient := autorest.NewClientWithUserAgent("")
	useragent.UpdateClient(&tokenClient)
	tokenClient.Sender = sender
	token.SetSender(&tokenClient)
	return token, tenantID, nil
}
