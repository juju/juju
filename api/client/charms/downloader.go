// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/http"
)

// CharmOpener provides the OpenCharm method.
type CharmOpener interface {
	OpenCharm(ctx context.Context, curl string) (io.ReadCloser, error)
}

type charmOpener struct {
	httpClient http.HTTPDoer
}

func (s *charmOpener) OpenCharm(ctx context.Context, curl string) (io.ReadCloser, error) {
	uri, query := openCharmArgs(curl)
	return http.OpenURI(ctx, s.httpClient, uri, query)
}

// NewCharmOpener returns a charm opener for the specified caller.
func NewCharmOpener(apiConn base.APICaller) (CharmOpener, error) {
	httpClient, err := apiConn.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &charmOpener{
		httpClient: httpClient,
	}, nil
}

func openCharmArgs(curl string) (string, url.Values) {
	query := make(url.Values)
	query.Add("url", curl)
	query.Add("file", "*")
	return "/charms", query
}
