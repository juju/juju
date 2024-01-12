// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	context "context"
	http "net/http"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	httprequest "gopkg.in/httprequest.v1"

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
	NewClient func(s3client.HTTPClient, s3client.Credentials, s3client.Logger) (objectstore.Session, error)

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
		Output: outputFunc,
		Start:  config.startFunc(),
	}
}

// startFunc returns a StartFunc that creates a S3 client based on the supplied
// manifold config and wraps it in a worker.
func (config ManifoldConfig) startFunc() dependency.StartFunc {
	return func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
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

		// TODO (stickupkid): Depending on the type of s3client we're using,
		// will then depend on the type of credentials we need to pass in.
		// For now, we're just using anonymous credentials, as we're just
		// hitting the local juju api server.
		credentials := s3client.AnonymousCredentials{}
		session, err := config.NewClient(newHTTPClient(httpClient), credentials, config.Logger)
		if err != nil {
			return nil, err
		}
		return newS3ClientWorker(session), nil
	}
}

// outputFunc extracts a S3 client from a *s3caller.
func outputFunc(in worker.Worker, out any) error {
	inWorker, _ := in.(*s3ClientWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *objectstore.Session:
		*outPointer = inWorker.session
	default:
		return errors.Errorf("out should be *s3caller.Session; got %T", out)
	}
	return nil
}

// NewS3Client returns a new S3 client based on the supplied dependencies.
func NewS3Client(client s3client.HTTPClient, creds s3client.Credentials, logger s3client.Logger) (objectstore.Session, error) {
	return s3client.NewS3Client(client, creds, logger)
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

func (c *httpClient) BaseURL() string {
	return c.client.BaseURL
}
