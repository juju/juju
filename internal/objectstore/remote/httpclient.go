// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"net/http"
	"strings"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/s3client"
	"gopkg.in/httprequest.v1"
)

// NewObjectClient returns a new client based on the supplied dependencies.
// This only provides a read only session to the object store. As this is
// intended to be used by the unit, there is never an expectation that the unit
// will write to the object store.
func NewObjectClient(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
	session, err := s3client.NewS3Client(ensureHTTPS(url), client, s3client.AnonymousCredentials{}, logger)
	if err != nil {
		return nil, err
	}

	return s3client.NewBlobsS3Client(session), nil
}

// httpClient is a shim around a shim. The httprequest.Client is a shim around
// the stdlib http.Client. This is just asinine. The httprequest.Client should
// be ripped out and replaced with the stdlib http.Client.
type httpClient struct {
	client *httprequest.Client
}

func newHTTPClient(client *httprequest.Client) *httpClient {
	return &httpClient{
		client: client,
	}
}

func (c *httpClient) Do(req *http.Request) (*http.Response, error) {
	var res *http.Response
	err := c.client.Do(req.Context(), req, &res)
	return res, err
}

// ensureHTTPS takes a URI and ensures that it is a HTTPS URL.
func ensureHTTPS(address string) string {
	if strings.HasPrefix(address, "https://") {
		return address
	}
	if strings.HasPrefix(address, "http://") {
		return strings.Replace(address, "http://", "https://", 1)
	}
	return "https://" + address
}
