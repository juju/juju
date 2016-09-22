// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/resources/subscriptions"
	"github.com/juju/errors"
	"github.com/juju/utils"
)

const authenticateHeaderKey = "WWW-Authenticate"

var authorizationUriRegexp = regexp.MustCompile(`authorization_uri="([^"]*)"`)

// DiscoverAuthorizationID returns the OAuth authorization URI for the given
// subscription ID. This can be used to determine the AD tenant ID.
func DiscoverAuthorizationURI(client subscriptions.Client, subscriptionID string) (*url.URL, error) {
	// We make an unauthenticated request to the Azure API, which
	// responds with the authentication URL with the tenant ID in it.
	result, err := client.Get(subscriptionID)
	if err == nil {
		return nil, errors.New("expected unauthorized error response")
	}
	if result.Response.Response == nil {
		return nil, errors.Trace(err)
	}
	if result.StatusCode != http.StatusUnauthorized {
		return nil, errors.Annotatef(err, "expected unauthorized error response, got %v", result.StatusCode)
	}

	header := result.Header.Get(authenticateHeaderKey)
	if header == "" {
		return nil, errors.Errorf("%s header not found", authenticateHeaderKey)
	}
	match := authorizationUriRegexp.FindStringSubmatch(header)
	if match == nil {
		return nil, errors.Errorf(
			"authorization_uri not found in %s header (%q)",
			authenticateHeaderKey, header,
		)
	}
	return url.Parse(match[1])
}

// AuthorizationURITenantID returns the tenant ID portion of the given URL,
// which is expected to have come from DiscoverAuthorizationURI.
func AuthorizationURITenantID(url *url.URL) (string, error) {
	path := strings.TrimPrefix(url.Path, "/")
	if _, err := utils.UUIDFromString(path); err != nil {
		return "", errors.NotValidf("authorization_uri %q", url)
	}
	return path, nil
}
