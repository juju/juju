// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/azureauth"
)

type OAuthConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&OAuthConfigSuite{})

const fakeTenantId = "11111111-1111-1111-1111-111111111111"

func oauthConfigSender() autorest.Sender {
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
	mocks.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{
		`authorization_uri="https://testing.invalid/` + fakeTenantId + `"`,
	})
	sender.AppendResponse(resp)
	return sender
}

func (s *OAuthConfigSuite) TestOAuthConfig(c *gc.C) {
	client := subscriptions.Client{subscriptions.NewWithBaseURI("https://testing.invalid")}
	client.Sender = oauthConfigSender()
	cfg, tenantId, err := azureauth.OAuthConfig(client, "https://testing.invalid", "subscription-id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tenantId, gc.Equals, fakeTenantId)

	baseURL := url.URL{
		Scheme:   "https",
		Host:     "testing.invalid",
		RawQuery: "api-version=1.0",
	}
	expectedCfg := &azure.OAuthConfig{
		AuthorizeEndpoint:  baseURL,
		TokenEndpoint:      baseURL,
		DeviceCodeEndpoint: baseURL,
	}
	expectedCfg.AuthorizeEndpoint.Path = "/11111111-1111-1111-1111-111111111111/oauth2/authorize"
	expectedCfg.TokenEndpoint.Path = "/11111111-1111-1111-1111-111111111111/oauth2/token"
	expectedCfg.DeviceCodeEndpoint.Path = "/11111111-1111-1111-1111-111111111111/oauth2/devicecode"

	c.Assert(cfg, jc.DeepEquals, expectedCfg)
}
