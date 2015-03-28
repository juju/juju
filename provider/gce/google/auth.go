// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/errors"
	"golang.org/x/oauth2"
	goauth2 "golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

var (
	driverScopes = []string{
		"https://www.googleapis.com/auth/compute",
		"https://www.googleapis.com/auth/devstorage.full_control",
	}
)

// newConnection opens a new low-level connection to the GCE API using
// the Auth's data and returns it. This includes building the
// OAuth-wrapping network transport.
func newConnection(creds *Credentials) (*compute.Service, error) {
	jsonKey := creds.JSONKey
	if jsonKey == nil {
		built, err := creds.buildJSONKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
		jsonKey = built
	}
	cfg, err := goauth2.JWTConfigFromJSON(jsonKey, driverScopes...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := cfg.Client(oauth2.NoContext)
	service, err := compute.New(client)
	return service, errors.Trace(err)
}
