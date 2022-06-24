// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package charms provides a client for accessing the charms API.
package charms

import (
	"io"
	"net/url"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/http"
	"github.com/juju/juju/downloader"
)

// NewCharmDownloader returns a new charm downloader that wraps the
// provided API caller.
func NewCharmDownloader(apiCaller base.APICaller) *downloader.Downloader {
	dlr := &downloader.Downloader{
		OpenBlob: func(url *url.URL) (io.ReadCloser, error) {
			curl, err := charm.ParseURL(url.String())
			if err != nil {
				return nil, errors.Annotate(err, "did not receive a valid charm URL")
			}
			reader, err := openCharm(apiCaller, curl)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return reader, nil
		},
	}
	return dlr
}

// CharmStreamerFunc returns a reader for the specified charm URL.
type CharmStreamerFunc func(curl *charm.URL) (io.ReadCloser, error)

// NewCharmStreamer returns a charm streamer func for the specified caller.
func NewCharmStreamer(apiCaller base.APICaller) CharmStreamerFunc {
	return func(curl *charm.URL) (io.ReadCloser, error) {
		uri, query := openCharmArgs(curl)
		return http.OpenURI(apiCaller, uri, query)
	}
}

// OpenCharm streams out the identified charm from the controller via
// the API.
func openCharm(apiCaller base.APICaller, curl *charm.URL) (io.ReadCloser, error) {
	uri, query := openCharmArgs(curl)
	return http.OpenURI(apiCaller, uri, query)
}

func openCharmArgs(curl *charm.URL) (string, url.Values) {
	query := make(url.Values)
	query.Add("url", curl.String())
	query.Add("file", "*")
	return "/charms", query
}
