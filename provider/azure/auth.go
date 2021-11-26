// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2016-06-01/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/juju/errors"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/provider/azure/internal/azureauth"
	"github.com/juju/juju/provider/azure/internal/useragent"
	"github.com/juju/juju/provider/common"
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
		token, resourceID, err := c.getToken(resourceID)
		if err != nil {
			return nil, errors.Annotatef(err, "constructing service principal token for resource %q", resourceID)
		}
		return autorest.NewBearerAuthorizer(token), nil
	})
}

func (c *cloudSpecAuth) refreshToken(resourceID string) error {
	token, resourceID, err := c.getToken(resourceID)
	if err != nil {
		return errors.Annotatef(err, "getting token to refresh for resource %q", resourceID)
	}
	return errors.Annotatef(token.Refresh(), "refreshing token for resource %q", resourceID)
}

func (c *cloudSpecAuth) purgeToken(resourceID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		return nil
	}

	if resourceID == "" {
		var err error
		resourceID, err = azureauth.ResourceManagerResourceId(c.cloud.StorageEndpoint)
		if err != nil {
			return errors.Trace(err)
		}
	}

	logger.Debugf("removing invalid token for %q", resourceID)
	delete(c.tokens, resourceID)
	return nil
}

func (c *cloudSpecAuth) getToken(resourceID string) (*adal.ServicePrincipalToken, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tokens == nil {
		c.tokens = make(map[string]*adal.ServicePrincipalToken)
	}

	if resourceID == "" {
		var err error
		resourceID, err = azureauth.ResourceManagerResourceId(c.cloud.StorageEndpoint)
		if err != nil {
			return nil, "", errors.Trace(err)
		}
	}

	if token := c.tokens[resourceID]; token != nil {
		return token, resourceID, nil
	}
	logger.Debugf("get auth token for %v", resourceID)
	token, tenantID, err := AuthToken(c.cloud, c.sender, resourceID)
	if err != nil {
		return nil, resourceID, errors.Annotatef(err, "constructing service principal token for resource %q", resourceID)
	}

	c.tokens[resourceID] = token
	c.tenantID = tenantID
	return token, resourceID, nil
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

	logger.Debugf("getting new auth token for resource %q", resourceID)
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
		func(t adal.Token) error {
			logger.Debugf("auth token refreshed for resource %q", resourceID)
			return nil
		},
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

// doRetryForStaleCredential is an autorest.SendDecorator which attempts to transparently
// recover from situations where the oauth token has become stale. It will first attempt
// to refresh the existing token, and if that fails, will get a brand new one using the
// current service principal credential.
func doRetryForStaleCredential(authorizer *cloudSpecAuth) autorest.SendDecorator {
	return func(s autorest.Sender) autorest.Sender {
		return autorest.SenderFunc(func(r *http.Request) (resp *http.Response, err error) {
			rr := autorest.NewRetriableRequest(r)
			// We make 3 attempts:
			// 1. the original request
			// 2. try again with a refreshed token
			// 3. try again with a brand new token
			for attempt := 1; attempt <= 3; attempt++ {
				var refreshErr error
				switch attempt {
				case 2:
					logger.Warningf("azure API call failed, attempting to refresh existing token: %v", err)
					refreshErr = authorizer.refreshToken("")
				case 3:
					logger.Warningf("azure API call failed, attempting to replace token: %v", err)
					refreshErr = authorizer.purgeToken("")
				}
				if refreshErr != nil {
					return nil, errors.Errorf("send request failed: %v\nand could not refresh oauth token: %v", err, refreshErr)
				}
				if err = rr.Prepare(); err != nil {
					return resp, errors.Trace(err)
				}
				if err = autorest.DrainResponseBody(resp); err != nil {
					return resp, errors.Trace(err)
				}
				resp, err = s.Do(rr.Request())
				// Exit if we get a non token refresh error.
				if err != nil && !autorest.IsTokenRefreshError(err) {
					break
				}
				// Exit if the response is not an auth issue.
				if err == nil && !autorest.ResponseHasStatusCode(resp, common.AuthorisationFailureStatusCodes.Values()...) {
					break
				}
			}
			return resp, errors.Trace(err)
		})
	}
}
