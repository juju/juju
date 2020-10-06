// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2016-06-01/subscriptions"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
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
	sdkCtx := context.Background()
	cfg, tenantId, err := azureauth.OAuthConfig(sdkCtx, client, "subscription-id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tenantId, gc.Equals, fakeTenantId)

	mustParseURL := func(s string) url.URL {
		u, err := url.Parse(s)
		c.Assert(err, jc.ErrorIsNil)
		return *u
	}

	baseURL := "https://testing.invalid/11111111-1111-1111-1111-111111111111"
	expectedCfg := &adal.OAuthConfig{
		AuthorityEndpoint:  mustParseURL(baseURL),
		AuthorizeEndpoint:  mustParseURL(baseURL + "/oauth2/authorize?api-version=1.0"),
		TokenEndpoint:      mustParseURL(baseURL + "/oauth2/token?api-version=1.0"),
		DeviceCodeEndpoint: mustParseURL(baseURL + "/oauth2/devicecode?api-version=1.0"),
	}
	c.Assert(cfg, jc.DeepEquals, expectedCfg)
}
