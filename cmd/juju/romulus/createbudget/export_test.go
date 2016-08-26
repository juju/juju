// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package createbudget

import (
	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var (
	NewAPIClient = &newAPIClient
)

func APIClientFnc(api apiClient) func(*httpbakery.Client) (apiClient, error) {
	return func(*httpbakery.Client) (apiClient, error) {
		return api, nil
	}
}
