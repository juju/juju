// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"

	"github.com/juju/errors"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/compute/v1"
)

// newComputeService opens a new low-level connection to the GCE API using
// the input credentials and returns it.
// This includes building the OAuth-wrapping network transport.
// TODO (manadart 2019-11-15) This the bottom layer in a cake of needless
// abstractions:
// This is embedded in rawConn, which is embedded in Connection,
// which is then used by the Environ.
// This should also be relocated alongside its wrapper,
// rather than this "auth.go" file.
func newComputeService(creds *Credentials) (*compute.Service, error) {
	jsonKey := creds.JSONKey
	if jsonKey == nil {
		built, err := creds.buildJSONKey()
		if err != nil {
			return nil, errors.Trace(err)
		}
		jsonKey = built
	}

	cfg, err := google.JWTConfigFromJSON(
		jsonKey,
		"https://www.googleapis.com/auth/compute",
		"https://www.googleapis.com/auth/devstorage.full_control",
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	client := cfg.Client(context.TODO())
	service, err := compute.New(client)
	return service, errors.Trace(err)
}
