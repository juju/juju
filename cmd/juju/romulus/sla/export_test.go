// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sla

import (
	"github.com/juju/romulus/api/sla"

	"github.com/juju/juju/api"
)

var (
	NewAuthorizationClient = &newAuthorizationClient
	NewSLAClient           = &newSlaClient
	ModelId                = &modelId
)

// APIClientFnc returns a function that returns the provided apiClient
// and can be used to patch the NewAPIClient variable for tests.
func APIClientFnc(client authorizationClient) func(...sla.ClientOption) (authorizationClient, error) {
	return func(...sla.ClientOption) (authorizationClient, error) {
		return client, nil
	}
}

func SLAClientFnc(client slaClient) func(api.Connection) slaClient {
	return func(api.Connection) slaClient {
		return client
	}
}

func ModelIdFnc(modelId string) func(api.Connection) string {
	return func(api.Connection) string {
		return modelId
	}
}
