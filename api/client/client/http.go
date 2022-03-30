// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/juju/api"
)

// openBlob streams the identified blob from the controller via the
// provided HTTP client.
func openBlob(ctx context.Context, httpClient api.HTTPDoer, endpoint string, args url.Values) (io.ReadCloser, error) {
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
