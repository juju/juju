// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/errors"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
)

var scopes = []string{
	"https://www.googleapis.com/auth/compute",
	"https://www.googleapis.com/auth/devstorage.full_control",
}

func newJWTConfig(creds *Credentials) (*jwt.Config, error) {
	jsonKey := creds.JSONKey
	if jsonKey == nil {
		var err error
		jsonKey, err = creds.buildJSONKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	return google.JWTConfigFromJSON(
		jsonKey,
		scopes...,
	)
}
