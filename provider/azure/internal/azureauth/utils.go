// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"net/url"
	"strings"
)

// ResourceManagerResourceId returns the resource ID for the
// Azure Resource Manager application to use in auth requests,
// based on the given core endpoint URI (e.g. https://core.windows.net).
//
// The core endpoint URI is the same as given in "storage-endpoint"
// in Azure cloud definitions, which serves as the suffix for blob
// storage URLs.
func ResourceManagerResourceId(coreEndpointURI string) (string, error) {
	u, err := url.Parse(coreEndpointURI)
	if err != nil {
		return "", err
	}
	u.Host = "management." + u.Host
	return TokenResource(u.String()), nil
}

// TokenResource returns a resource value suitable for auth tokens, based on
// an endpoint URI.
func TokenResource(uri string) string {
	resource := uri
	if !strings.HasSuffix(resource, "/") {
		resource += "/"
	}
	return resource
}
