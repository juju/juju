// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/google-api-go-client/compute/v1"

	"github.com/juju/juju/environs"
)

var (
	Provider   environs.EnvironProvider = providerInstance
	NewToken                            = &newToken
	NewService                          = &newService
)

func DummyNewToken(gceAuth, string) (*oauth.Token, error) {
	return nil, nil
}

func DummyNewService(*oauth.Transport) (*compute.Service, error) {
	return nil, nil
}
