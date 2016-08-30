// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setplan

import (
	api "github.com/juju/romulus/api/plan"
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
