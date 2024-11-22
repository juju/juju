// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
)

const authenticateHeaderKey = "WWW-Authenticate"

// recordAuthHeaderPolicy records the value of a http response auth header.
type recordAuthHeaderPolicy struct {
	authHeader string
}

func (p *recordAuthHeaderPolicy) Do(req *policy.Request) (*http.Response, error) {
	resp, err := req.Next()
	if resp.Header != nil {
		p.authHeader = resp.Header.Get(authenticateHeaderKey)
	}
	return resp, err
}

// fakeCredential returns an invalid token to trigger
// a response with the WWW-Authenticate header.
type fakeCredential struct{}

// GetToken provide a fake access token.
func (c *fakeCredential) GetToken(ctx context.Context, opts policy.TokenRequestOptions) (azcore.AccessToken, error) {
	return azcore.AccessToken{Token: "FakeToken"}, nil
}

var authorizationUriRegexp = regexp.MustCompile(`authorization_uri="([^"]*)"`)

// DiscoverTenantID returns the tenent ID for the given
// subscription ID.
func DiscoverTenantID(ctx context.Context, subscriptionID string, clientOpts arm.ClientOptions) (string, error) {
	// We make an unauthenticated request to the Azure API, which
	// responds with the authentication URL with the tenant ID in it.

	authRecorder := &recordAuthHeaderPolicy{}
	clientOpts.PerCallPolicies = append(clientOpts.PerCallPolicies, authRecorder)

	client, err := armsubscriptions.NewClient(&fakeCredential{}, &clientOpts)
	if err != nil {
		return "", errors.Trace(err)
	}

	_, err = client.Get(ctx, subscriptionID, nil)
	if err == nil {
		return "", errors.New("expected unauthorized error response")
	}
	if code := errorutils.StatusCode(err); code != http.StatusUnauthorized {
		return "", errors.Annotatef(err, "expected unauthorized error response, got %v", code)
	}

	header := authRecorder.authHeader
	if header == "" {
		return "", errors.Errorf("%s header not found", authenticateHeaderKey)
	}
	match := authorizationUriRegexp.FindStringSubmatch(header)
	if match == nil {
		return "", errors.Errorf(
			"authorization_uri not found in %s header (%q)",
			authenticateHeaderKey, header,
		)
	}

	authURL, err := url.Parse(match[1])
	if err != nil {
		return "", errors.Annotatef(err, "cannot parse auth URL %q", match[1])
	}

	// Get the tenant ID portion of the auth URL.
	path := strings.TrimPrefix(authURL.Path, "/")
	if _, err := utils.UUIDFromString(path); err != nil {
		return "", errors.NotValidf("authorization_uri %q", authURL)
	}
	return path, nil
}
