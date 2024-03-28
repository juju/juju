// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// CharmPutter uploads a local charm blob to the controller
type CharmPutter interface {
	PutCharm(ctx context.Context, modelUUID, charmRef, curl string, body io.Reader) (string, error)
}

// httpPutter uploads local charm blobs to the controller via the legacy
// "model/:modeluuid/charms" endpoint hosted by the controller
type httpPutter struct {
	httpClient *httprequest.Client
}

// newHTTPPutter create an httpPutter, which uploads local charm blobs
// to the controller via the legacy "model/:modeluuid/charms" endpoint
// hosted by the controller
func newHTTPPutter(api base.APICaller) (CharmPutter, error) {
	// The returned httpClient sets the base url to /model/:modeluuid if it can.
	apiHTTPClient, err := api.HTTPClient()
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve http client from the api connection")
	}
	return &httpPutter{
		httpClient: apiHTTPClient,
	}, nil
}

func (h *httpPutter) PutCharm(ctx context.Context, _, _, curlStr string, body io.Reader) (string, error) {
	curl, err := charm.ParseURL(curlStr)
	if err != nil {
		return "", errors.Trace(err)
	}
	args := url.Values{}
	args.Add("series", curl.Series)
	args.Add("schema", curl.Schema)
	args.Add("revision", strconv.Itoa(curl.Revision))
	apiURI := url.URL{Path: "/charms", RawQuery: args.Encode()}

	var resp params.CharmsResponse
	req, err := http.NewRequest("POST", apiURI.String(), body)
	if err != nil {
		return "", errors.Annotate(err, "cannot create upload request")
	}
	req.Header.Set("Content-Type", "application/zip")

	if err := h.httpClient.Do(ctx, req, &resp); err != nil {
		return "", errors.Trace(err)
	}
	return resp.CharmURL, nil
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

// fallbackPutter iterates over a number of sub-putters, attempting to upload a charm
// blob until one is successful. If a putter fails, before falling back to the next one
// we check is the error is 'fallback-able'. It doesn't make sense to fallback on certain
// error types such as unauthorised
type fallbackPutter struct {
	putters []CharmPutter
}

// newFallbackPutter creates a fallbackPutter with at least 2 sub-putters
func newFallbackPutter(putters ...CharmPutter) (CharmPutter, error) {
	if len(putters) == 0 {
		return nil, errors.Errorf("programming error: fallbackPutter requires at least 1 sub putter")
	}
	return &fallbackPutter{
		putters: putters,
	}, nil
}

func (h *fallbackPutter) PutCharm(ctx context.Context, modelUUID, charmRef, curl string, b io.Reader) (string, error) {
	// body must be a ReadSeeker to use PutCharm on fallbackPutter so we can rewind the body
	// between requests
	body, ok := b.(io.ReadSeeker)
	if !ok {
		return "", errors.Errorf("Programming error: body must be a seeker to use FallbackPutter")
	}

	// Wrap our body in a nopCloser to ensure sub-putters do not close our body before passing to the next
	// putter.  As a consequence, we need to defer a close ourselves, if closeable.
	nopCloserBody := io.NopCloser(body)
	if closer, ok := body.(io.Closer); ok {
		defer closer.Close()
	}

	var err error
	for _, putter := range h.putters {
		respCurl, err := putter.PutCharm(ctx, modelUUID, charmRef, curl, nopCloserBody)
		if err == nil {
			return respCurl, nil
		}
		if !h.fallbackableError(err) {
			return "", errors.Trace(err)
		}
		_, seekErr := body.Seek(0, os.SEEK_SET)
		if seekErr != nil {
			return "", errors.Trace(seekErr)
		}
	}
	return "", errors.Annotate(err, "All charm putters failed")
}

func (h *fallbackPutter) fallbackableError(err error) bool {
	fallbackableTypes := []errors.ConstError{errors.NotFound, errors.MethodNotAllowed}
	for _, typ := range fallbackableTypes {
		if errors.Is(errors.Cause(err), typ) {
			return true
		}
	}
	return false
}
