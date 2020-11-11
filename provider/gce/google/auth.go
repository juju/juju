// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"context"
	"net/http"
	"time"

	"github.com/juju/errors"
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
func newComputeService(creds *Credentials) (*compute.Service, error) {
	cfg, err := newJWTConfig(creds)
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := context.Background()

	ts := cfg.TokenSource(ctx)
	tsOpt := option.WithTokenSource(ts)

	newTransport := http.DefaultTransport.(*http.Transport).Clone()
	newTransport.TLSHandshakeTimeout = 20 * time.Second

	httpTransport, err := transporthttp.NewTransport(ctx, newTransport, tsOpt)
	if err != nil {
		return nil, errors.Trace(err)
	}

	httpClient := &http.Client{}
	httpClient.Transport = httpTransport

	service, err := compute.NewService(ctx,
		tsOpt,
		option.WithHTTPClient(httpClient),
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
