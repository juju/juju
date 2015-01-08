// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/goauth2/oauth"
	"code.google.com/p/google-api-go-client/compute/v1"
)

var (
	NewToken   = &newToken
	NewService = &newService
)

func DummyNewToken(Auth, string) (*oauth.Token, error) {
	return nil, nil
}

func DummyNewService(*oauth.Transport) (*compute.Service, error) {
	return nil, nil
}
