// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"

	"github.com/juju/errors"
	jujuhttp "github.com/juju/juju/internal/http"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	transporthttp "google.golang.org/api/transport/http"
)

var scopes = []string{
	"https://www.googleapis.com/auth/compute",
	"https://www.googleapis.com/auth/devstorage.full_control",
}

// newComputeService opens a new low-level connection to the GCE API using
// the input credentials and returns it.
// This includes building the OAuth-wrapping network transport.
// TODO (manadart 2019-11-15) This the bottom layer in a cake of needless
// abstractions:
// This is embedded in rawConn, which is embedded in Connection,
// which is then used by the Environ.
// This should also be relocated alongside its wrapper,
// rather than this "auth.go" file.
func newComputeService(ctx context.Context, creds *Credentials, httpClient *jujuhttp.Client) (*compute.Service, error) {
	cfg, err := newJWTConfig(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We're substituting the transport, with a wrapped GCE specific version of
	// the original http.Client.
	newClient := *httpClient.Client()

	tsOpt := option.WithTokenSource(cfg.TokenSource(ctx))
	if newClient.Transport, err = transporthttp.NewTransport(ctx, newClient.Transport, tsOpt); err != nil {
		return nil, errors.Trace(err)
	}

	service, err := compute.NewService(ctx,
		tsOpt,
		option.WithHTTPClient(&newClient),
	)
	return service, errors.Trace(err)
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
