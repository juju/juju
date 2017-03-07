// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package support

import (
	api "github.com/juju/romulus/api/support"
)

var (
	NewAuthorizationClient = &newAuthorizationClient
)

// APIClientFnc returns a function that returns the provided apiClient
// and can be used to patch the NewAPIClient variable for tests.
func APIClientFnc(client authorizationClient) func(...api.ClientOption) (authorizationClient, error) {
	return func(...api.ClientOption) (authorizationClient, error) {
		return client, nil
	}
}
