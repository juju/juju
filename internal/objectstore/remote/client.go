// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remote

import (
	"net/http"
	"strings"

	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/s3client"
)

// NewBlobsClient returns a new client based on the supplied dependencies.
// This only provides a read only session to the object store. As this is
// intended to be used by the unit, there is never an expectation that the unit
// will write to the object store.
func NewBlobsClient(url string, client s3client.HTTPClient, logger logger.Logger) (BlobsClient, error) {
	creds := s3client.AnonymousCredentials{}
	session, err := s3client.NewS3Client(ensureHTTPS(url), client, creds,
		s3client.WithLogger(logger),

		// We don't want s3 to retry requests as there already exists a set
		// of retry logic in the apiremotecaller and it can cause exponential
		// backoff which can cause long delays in retrieving blobs.
		s3client.WithMaxAttempts(1),

		// We also don't want rate limiting from the client side, that should
		// be handled by the apiserver.
		s3client.WithRateLimiting(false),
	)
	if err != nil {
		return nil, err
	}

	return s3client.NewBlobsS3Client(session), nil
}

// httpClient is a shim around a shim. The httprequest.Client is a shim around
// the stdlib http.Client. This is just asinine. The httprequest.Client should
// be ripped out and replaced with the stdlib http.Client.
type httpClient struct {
	client httprequest.Doer
}

func newHTTPClient(client httprequest.Doer) *httpClient {
	return &httpClient{
		client: client,
	}
}

func (c *httpClient) Do(req *http.Request) (*http.Response, error) {
	res, err := c.client.Do(req)
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
