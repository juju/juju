// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/charm/v11"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/http"
	"github.com/juju/juju/downloader"
)

// NewCharmDownloader returns a new charm downloader that wraps the
// provided API caller.
func NewCharmDownloader(apiCaller base.APICaller) *downloader.Downloader {
	dlr := &downloader.Downloader{
		OpenBlob: func(req downloader.Request) (io.ReadCloser, error) {
			curl, err := charm.ParseURL(req.URL.String())
			if err != nil {
				return nil, errors.Annotate(err, "did not receive a valid charm URL")
			}
			streamer, err := NewCharmOpener(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			reader, err := streamer.OpenCharm(curl)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return reader, nil
		},
	}
	return dlr
}

// CharmOpener provides the OpenCharm method.
type CharmOpener interface {
	OpenCharm(curl *charm.URL) (io.ReadCloser, error)
}

type charmOpener struct {
	ctx        context.Context
	httpClient http.HTTPDoer
}

func (s *charmOpener) OpenCharm(curl *charm.URL) (io.ReadCloser, error) {
	uri, query := openCharmArgs(curl)
	return http.OpenURI(s.ctx, s.httpClient, uri, query)
}

// NewCharmOpener returns a charm opener for the specified caller.
func NewCharmOpener(apiConn base.APICaller) (CharmOpener, error) {
	httpClient, err := apiConn.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &charmOpener{
		ctx:        apiConn.Context(),
		httpClient: httpClient,
	}, nil
}

func openCharmArgs(curl *charm.URL) (string, url.Values) {
	query := make(url.Values)
	query.Add("url", curl.String())
	query.Add("file", "*")
	return "/charms", query
}
