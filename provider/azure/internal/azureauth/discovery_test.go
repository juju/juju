// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"net/http"
	"net/url"

	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/azure/internal/azureauth"
)

type DiscoverySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&DiscoverySuite{})

func (*DiscoverySuite) TestDiscoverAuthorizationURI(c *gc.C) {
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
	mocks.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{
		`foo bar authorization_uri="https://testing.invalid/meep" baz`,
	})
	sender.AppendResponse(resp)

	client := subscriptions.NewClient()
	client.Sender = sender
	authURI, err := azureauth.DiscoverAuthorizationURI(client, "subscription_id")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authURI, jc.DeepEquals, &url.URL{
		Scheme: "https",
		Host:   "testing.invalid",
		Path:   "/meep",
	})
}

func (*DiscoverySuite) TestDiscoverAuthorizationURIMissingHeader(c *gc.C) {
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
	sender.AppendResponse(resp)

	client := subscriptions.NewClient()
	client.Sender = sender
	_, err := azureauth.DiscoverAuthorizationURI(client, "subscription_id")
	c.Assert(err, gc.ErrorMatches, `WWW-Authenticate header not found`)
}

func (*DiscoverySuite) TestDiscoverAuthorizationURIHeaderMismatch(c *gc.C) {
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
	mocks.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{`foo bar baz`})
	sender.AppendResponse(resp)

	client := subscriptions.NewClient()
	client.Sender = sender
	_, err := azureauth.DiscoverAuthorizationURI(client, "subscription_id")
	c.Assert(err, gc.ErrorMatches, `authorization_uri not found in WWW-Authenticate header \("foo bar baz"\)`)
}

func (*DiscoverySuite) TestDiscoverAuthorizationURIUnexpectedSuccess(c *gc.C) {
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusOK)
	sender.AppendResponse(resp)

	client := subscriptions.NewClient()
	client.Sender = sender
	_, err := azureauth.DiscoverAuthorizationURI(client, "subscription_id")
	c.Assert(err, gc.ErrorMatches, "expected unauthorized error response")
}

func (*DiscoverySuite) TestDiscoverAuthorizationURIUnexpectedStatusCode(c *gc.C) {
	sender := mocks.NewSender()
	resp := mocks.NewResponseWithStatus("", http.StatusNotFound)
	sender.AppendResponse(resp)

	client := subscriptions.NewClient()
	client.Sender = sender
	_, err := azureauth.DiscoverAuthorizationURI(client, "subscription_id")
	c.Assert(err, gc.ErrorMatches, "expected unauthorized error response, got 404: .*")
}

func (*DiscoverySuite) TestAuthorizationURITenantID(c *gc.C) {
	tenantId, err := azureauth.AuthorizationURITenantID(&url.URL{Path: "/3671f5a9-c0d0-472b-a80c-48135cf5a9f1"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(tenantId, gc.Equals, "3671f5a9-c0d0-472b-a80c-48135cf5a9f1")
}

func (*DiscoverySuite) TestAuthorizationURITenantIDError(c *gc.C) {
	url, err := url.Parse("https://testing.invalid/foo")
	c.Assert(err, jc.ErrorIsNil)
	_, err = azureauth.AuthorizationURITenantID(url)
	c.Assert(err, gc.ErrorMatches, `authorization_uri "https://testing.invalid/foo" not valid`)
}
