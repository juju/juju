// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listwallets

import (
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
)

var (
	NewAPIClient = &newAPIClient
)

// APIClientFnc returns a function that returns the provided apiClient
// and can be used to patch the NewAPIClient variable for tests.
func APIClientFnc(api apiClient) func(string, *httpbakery.Client) (apiClient, error) {
	return func(string, *httpbakery.Client) (apiClient, error) {
		return api, nil
	}
}
