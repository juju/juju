// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package units3caller

import (
	context "context"
	http "net/http"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	httprequest "gopkg.in/httprequest.v1"

	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/api"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/s3client"
)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {
	// APICallerName is the name of the APICaller resource that
	// supplies the API connection.
	APICallerName string

	// NewClient is used to create a new object store client.
	NewClient func(string, s3client.HTTPClient, s3client.Logger) (objectstore.ReadSession, error)

	// Logger is used to write logging statements for the worker.
	Logger s3client.Logger
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.APICallerName == "" {
		return errors.NotValidf("nil APICallerName")
	}
	if cfg.NewClient == nil {
		return errors.NotValidf("nil NewClient")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a manifold whose worker wraps an S3 Session.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.APICallerName,
		},
		Output: engine.ValueWorkerOutput,
		Start:  config.start,
	}
}

// start returns a StartFunc that creates a S3 client based on the supplied
// manifold config and wraps it in a worker.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiConn api.Connection
	if err := getter.Get(config.APICallerName, &apiConn); err != nil {
		return nil, err
	}

	httpClient, err := apiConn.RootHTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Although we get the S3 client, this is the anonymous client which only
	// provides read access to the object store.
	session, err := config.NewClient(httpClient.BaseURL, newHTTPClient(httpClient), config.Logger)
	if err != nil {
		return nil, err
	}
	return engine.NewValueWorker(session)
}

// NewS3Client returns a new S3 client based on the supplied dependencies.
// This only provides a read only session to the object store. As this is
// intended to be used by the unit, there is never an expectation that the unit
// will write to the object store.
func NewS3Client(url string, client s3client.HTTPClient, logger s3client.Logger) (objectstore.ReadSession, error) {
	return s3client.NewS3Client(url, client, s3client.AnonymousCredentials{}, logger)
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
