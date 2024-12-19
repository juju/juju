// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
)

// CharmPutter uploads a local charm blob to the controller
type CharmPutter interface {
	PutCharm(ctx context.Context, modelUUID, charmRef, curl string, body io.Reader) (string, error)
}

// s3Putter uploads local charm blobs to the controller via the
// s3-compatible api endpoint "model-:modeluuid/charms/:object"
// We use a regular httpClient to curl the s3-compatible endpoint
type s3Putter struct {
	httpClient *httprequest.Client
}

// newS3Putter creates an s3Putter, which uploads local charm blobs
// to the controller via the s3-compatible api endpoint
// "model-:modeluuid/charms/:object"
func newS3Putter(api base.APICaller) (CharmPutter, error) {
	apiHTTPClient, err := api.RootHTTPClient()
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve http client from the api connection")
	}
	return &s3Putter{
		httpClient: apiHTTPClient,
	}, nil
}

func (h *s3Putter) PutCharm(ctx context.Context, modelUUID, charmRef, curl string, body io.Reader) (string, error) {
	apiURI := url.URL{Path: fmt.Sprintf("/model-%s/charms/%s", modelUUID, charmRef)}

	resp := &http.Response{}
	req, err := http.NewRequest("PUT", apiURI.String(), body)
	if err != nil {
		return "", errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Juju-Curl", curl)

	if err := h.httpClient.Do(ctx, req, &resp); err != nil {
		return "", errors.Trace(err)
	}

	return resp.Header.Get("Juju-Curl"), nil
}
