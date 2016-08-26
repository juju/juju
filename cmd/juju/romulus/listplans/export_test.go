// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listplans

import (
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	NewClient = &newClient
)

// APIClientFnc returns a function that returns the provided apiClient
// and can be used to patch the NewAPIClient variable for tests.
func APIClientFnc(api apiClient) func(client *httpbakery.Client) (apiClient, error) {
	return func(*httpbakery.Client) (apiClient, error) {
		return api, nil
	}
}
