// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listbudgets

import (
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	NewAPIClient = &newAPIClient
)

// APIClientFnc returns a function that returns the provided apiClient
// and can be used to patch the NewAPIClient variable for tests.
func APIClientFnc(api apiClient) func(*httpbakery.Client) (apiClient, error) {
	return func(*httpbakery.Client) (apiClient, error) {
		return api, nil
	}
}
