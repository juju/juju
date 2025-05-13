// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type DiscoverySuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&DiscoverySuite{})

//func (*DiscoverySuite) TestDiscoverAuthorizationURI(c *tc.C) {
//	sender := mocks.NewSender()
//	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
//	mocks.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{
//		`foo bar authorization_uri="https://testing.invalid/meep" baz`,
//	})
//	sender.AppendResponse(resp)
//
//	client := armsubscriptions.NewClient()
//	sdkCtx := context.Background()
//	client.Sender = sender
//	authURI, err := azureauth.DiscoverTenantID(sdkCtx, client, "subscription_id")
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(authURI, tc.DeepEquals, &url.URL{
//		Scheme: "https",
//		Host:   "testing.invalid",
//		Path:   "/meep",
//	})
//}
//
//func (*DiscoverySuite) TestDiscoverAuthorizationURIMissingHeader(c *tc.C) {
//	sender := mocks.NewSender()
//	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
//	sender.AppendResponse(resp)
//
//	client := subscriptions.NewClient()
//	client.Sender = sender
//	sdkCtx := context.Background()
//	_, err := azureauth.DiscoverAuthorizationURI(sdkCtx, client, "subscription_id")
//	c.Assert(err, tc.ErrorMatches, `WWW-Authenticate header not found`)
//}
//
//func (*DiscoverySuite) TestDiscoverAuthorizationURIHeaderMismatch(c *tc.C) {
//	sender := mocks.NewSender()
//	resp := mocks.NewResponseWithStatus("", http.StatusUnauthorized)
//	mocks.SetResponseHeaderValues(resp, "WWW-Authenticate", []string{`foo bar baz`})
//	sender.AppendResponse(resp)
//
//	client := subscriptions.NewClient()
//	client.Sender = sender
//	sdkCtx := context.Background()
//	_, err := azureauth.DiscoverAuthorizationURI(sdkCtx, client, "subscription_id")
//	c.Assert(err, tc.ErrorMatches, `authorization_uri not found in WWW-Authenticate header \("foo bar baz"\)`)
//}
//
//func (*DiscoverySuite) TestDiscoverAuthorizationURIUnexpectedSuccess(c *tc.C) {
//	sender := mocks.NewSender()
//	resp := mocks.NewResponseWithStatus("", http.StatusOK)
//	sender.AppendResponse(resp)
//
//	client := subscriptions.NewClient()
//	client.Sender = sender
//	sdkCtx := context.Background()
//	_, err := azureauth.DiscoverAuthorizationURI(sdkCtx, client, "subscription_id")
//	c.Assert(err, tc.ErrorMatches, "expected unauthorized error response")
//}
//
//func (*DiscoverySuite) TestDiscoverAuthorizationURIUnexpectedStatusCode(c *tc.C) {
//	sender := mocks.NewSender()
//	resp := mocks.NewResponseWithStatus("", http.StatusNotFound)
//	sender.AppendResponse(resp)
//
//	client := subscriptions.NewClient()
//	client.Sender = sender
//	sdkCtx := context.Background()
//	_, err := azureauth.DiscoverAuthorizationURI(sdkCtx, client, "subscription_id")
//	c.Assert(err, tc.ErrorMatches, "expected unauthorized error response, got 404: .*")
//}
//
//func (*DiscoverySuite) TestAuthorizationURITenantID(c *tc.C) {
//	tenantId, err := azureauth.AuthorizationURITenantID(&url.URL{Path: "/3671f5a9-c0d0-472b-a80c-48135cf5a9f1"})
//	c.Assert(err, tc.ErrorIsNil)
//	c.Assert(tenantId, tc.Equals, "3671f5a9-c0d0-472b-a80c-48135cf5a9f1")
//}
//
//func (*DiscoverySuite) TestAuthorizationURITenantIDError(c *tc.C) {
//	url, err := url.Parse("https://testing.invalid/foo")
//	c.Assert(err, tc.ErrorIsNil)
//	_, err = azureauth.AuthorizationURITenantID(url)
//	c.Assert(err, tc.ErrorMatches, `authorization_uri "https://testing.invalid/foo" not valid`)
//}
