// Copyright 2014 ALTOROS
// Licensed under the AGPLv3, see LICENSE file for details.

package gosigma

import (
	"errors"
	"fmt"
	"net/url"
)

// Default CloudSigma region
const DefaultRegion string = "zrh"

var errEmptyEndpoint = errors.New("endpoint are not allowed to be empty")
var errHttpsRequired = errors.New("endpoint must use https scheme")
var errInvalidAuth = errors.New("auth information is not allowed in the endpoint string")
var errEndpointWithQuery = errors.New("query information is not allowed in the endpoint string")

// ResolveEndpoint returns endpoint for given region code
func ResolveEndpoint(endpoint string) string {
	if err := VerifyEndpoint(endpoint); err == nil {
		return endpoint
	}
	return fmt.Sprintf("https://%s.cloudsigma.com/api/2.0/", endpoint)
}

// VerifyEndpoint verifies CloudSigma endpoint URL
func VerifyEndpoint(e string) error {
	if len(e) == 0 {
		return errEmptyEndpoint
	}

	u, err := url.Parse(e)
	if err != nil {
		return err
	}

	if u.Scheme != "https" {
		return errHttpsRequired
	}

	if u.User != nil {
		return errInvalidAuth
	}

	if len(u.RawQuery) > 0 || len(u.Fragment) > 0 {
		return errEndpointWithQuery
	}

	return nil
}
