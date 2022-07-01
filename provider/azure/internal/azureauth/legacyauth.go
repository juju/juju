// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"sync"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2016-06-01/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/errors"

	environscloudspec "github.com/juju/juju/v2/environs/cloudspec"
	"github.com/juju/juju/v2/provider/azure/internal/useragent"
)

// NewLegacyAuth creates a legacy auth for the specified cloud.
// Only used with the legacy unmanaged storage API client which
// is deprecated by Azure and removed in Juju 3.0
func NewLegacyAuth(cloud environscloudspec.CloudSpec, tenantID string) *cloudSpecAuth {
	return &cloudSpecAuth{
		cloud:    cloud,
		tenantID: tenantID,
	}
}

// cloudSpecAuth provides an implementation of autorest.Authorizer.
type cloudSpecAuth struct {
	cloud    environscloudspec.CloudSpec
	sender   autorest.Sender
	mu       sync.Mutex
	tokens   map[string]*adal.ServicePrincipalToken
	tenantID string
}

func (c *cloudSpecAuth) Auth() *autorest.BearerAuthorizerCallback {
	return autorest.NewBearerAuthorizerCallback(c.sender, func(tenantID, resourceID string) (*autorest.BearerAuthorizer, error) {
		token, resourceID, err := c.getToken(resourceID)
		if err != nil {
			return nil, errors.Annotatef(err, "constructing service principal token for resource %q", resourceID)
		}
		return autorest.NewBearerAuthorizer(token), nil
	})
}

func (c *cloudSpecAuth) getToken(resourceID string) (*adal.ServicePrincipalToken, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = make(map[string]*adal.ServicePrincipalToken)
	}

	if resourceID == "" {
		var err error
		resourceID, err = ResourceManagerResourceId(c.cloud.StorageEndpoint)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
	}

	if token := c.tokens[resourceID]; token != nil {
		return token, resourceID, nil
	}
	logger.Debugf("get auth token for %v", resourceID)
	token, err := AuthToken(c.cloud, c.sender, resourceID, c.tenantID)
	if err != nil {
		return nil, resourceID, errors.Annotatef(err, "constructing service principal token for resource %q", resourceID)
	}

	c.tokens[resourceID] = token
	return token, resourceID, nil
}

// AuthToken returns a service principal token, suitable for authorizing
// Resource Manager API requests, based on the supplied CloudSpec.
func AuthToken(cloud environscloudspec.CloudSpec, sender autorest.Sender, resourceID, tenantID string) (*adal.ServicePrincipalToken, error) {
	if authType := cloud.Credential.AuthType(); authType != "service-principal-secret" {
		// We currently only support a single auth-type for
		// non-interactive authentication. Interactive auth
		// is used only to generate a service-principal.
		return nil, errors.NotSupportedf("auth-type %q", authType)
	}

	logger.Debugf("getting new auth token for resource %q", resourceID)
	credAttrs := cloud.Credential.Attributes()
	appId := credAttrs["application-id"]
	appPassword := credAttrs["application-password"]
	client := subscriptions.Client{subscriptions.NewWithBaseURI(cloud.Endpoint)}
	useragent.UpdateClient(&client.Client)
	client.Sender = sender
	oauthConfig, err := adal.NewOAuthConfig("https://login.windows.net", tenantID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	token, err := adal.NewServicePrincipalToken(
		*oauthConfig,
		appId,
		appPassword,
		resourceID,
		func(t adal.Token) error {
			logger.Debugf("auth token refreshed for resource %q", resourceID)
			return nil
		},
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
