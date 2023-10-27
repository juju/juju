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
	"github.com/juju/juju/downloader"
)

// NewCharmDownloader returns a new charm downloader that wraps the
// provided API caller.
func NewCharmDownloader(apiCaller base.APICaller) *downloader.Downloader {
	dlr := &downloader.Downloader{
		OpenBlob: func(req downloader.Request) (io.ReadCloser, error) {
			streamer, err := NewCharmOpener(apiCaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			reader, err := streamer.OpenCharm(req.URL.String())
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
	OpenCharm(curl string) (io.ReadCloser, error)
}

type charmOpener struct {
	ctx        context.Context
	httpClient http.HTTPDoer
}

func (s *charmOpener) OpenCharm(curl string) (io.ReadCloser, error) {
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

func openCharmArgs(curl string) (string, url.Values) {
	query := make(url.Values)
	query.Add("url", curl)
	query.Add("file", "*")
	return "/charms", query
}
