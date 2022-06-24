// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package http

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
)

// HTTPClient is an API-specific HTTP client.
type HTTPClient interface {
	// Do sends the HTTP request, returning the subsequent response.
	Do(req *http.Request) (*http.Response, error)
}

// HTTPDoer exposes the functionality of httprequest.Client needed here.
type HTTPDoer interface {
	// Do sends the given request.
	Do(context context.Context, req *http.Request, resp interface{}) error
}

// OpenURIFunc returns a reader for the blob at the specified uri.
type OpenURIFunc func(uri string, query url.Values) (io.ReadCloser, error)

// NewURIOpener returns a URI opener func for the api caller.
func NewURIOpener(apiCaller base.APICaller) OpenURIFunc {
	return func(uri string, query url.Values) (io.ReadCloser, error) {
		return OpenURI(apiCaller, uri, query)
	}
}

// OpenURI performs a GET on a Juju HTTP endpoint returning the specified blob.
func OpenURI(apiCaller base.APICaller, uri string, query url.Values) (io.ReadCloser, error) {
	// The returned httpClient sets the base url to /model/<uuid> if it can.
	httpClient, err := apiCaller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	blob, err := openBlob(apiCaller.Context(), httpClient, uri, query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return blob, nil
}

// openBlob streams the identified blob from the controller via the
// provided HTTP client.
func openBlob(ctx context.Context, httpClient HTTPDoer, endpoint string, args url.Values) (io.ReadCloser, error) {
	apiURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiURL.RawQuery = args.Encode()
	req, err := http.NewRequest("GET", apiURL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create HTTP request")
	}

	var resp *http.Response
	if err := httpClient.Do(ctx, req, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.Body, nil
}
