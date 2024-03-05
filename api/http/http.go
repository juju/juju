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

// URIOpener provides the OpenURI method.
type URIOpener interface {
	OpenURI(ctx context.Context, uri string, query url.Values) (io.ReadCloser, error)
}

type uriOpener struct {
	httpClient HTTPDoer
}

// OpenURI performs a GET on a Juju HTTP endpoint returning the specified blob.
func (o *uriOpener) OpenURI(ctx context.Context, uri string, query url.Values) (io.ReadCloser, error) {
	return OpenURI(ctx, o.httpClient, uri, query)
}

// NewURIOpener returns a URI opener for the api caller.
func NewURIOpener(apiConn base.APICaller) (URIOpener, error) {
	httpClient, err := apiConn.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &uriOpener{
		httpClient: httpClient,
	}, nil
}

// OpenURI performs a GET on a Juju HTTP endpoint returning the specified blob.
func OpenURI(ctx context.Context, httpClient HTTPDoer, uri string, query url.Values) (io.ReadCloser, error) {
	blob, err := openBlobReader(ctx, httpClient, uri, query)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return blob, nil
}

// openBlobReader streams the identified blob from the controller via the
// provided HTTP client.
func openBlobReader(ctx context.Context, httpClient HTTPDoer, endpoint string, args url.Values) (io.ReadCloser, error) {
	apiURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiURL.RawQuery = args.Encode()
	req, err := http.NewRequest(http.MethodGet, apiURL.String(), nil)
	if err != nil {
		return nil, errors.Annotate(err, "cannot create HTTP request")
	}

	var resp *http.Response
	if err := httpClient.Do(ctx, req, &resp); err != nil {
		return nil, errors.Trace(err)
	}
	return resp.Body, nil
}
