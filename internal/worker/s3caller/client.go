// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/objectstore"
)

type objectsClient struct {
	logger Logger
	client *s3.Client
}

// GetObject gets an object from the object store based on the bucket name and
// object name.
func (c *objectsClient) GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	c.logger.Tracef("retrieving bucket %s object %s from s3 storage", bucketName, objectName)

	obj, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucketName),
		Key:    aws.String(objectName),
	},
	)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to get object %s on bucket %s using S3 client", objectName, bucketName)
	}
	return obj.Body, nil
}

type awsEndpointResolver struct {
	endpoint string
}

// ResolveEndpoint returns the endpoint for the given service and region.
func (a *awsEndpointResolver) ResolveEndpoint(service, region string) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL: a.endpoint,
	}, nil
}

type awsHTTPDoer struct {
	client *httprequest.Client
}

func (c *awsHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	var res *http.Response
	err := c.client.Do(req.Context(), req, &res)
	return res, err
}

type awsLogger struct {
	logger Logger
}

func (l *awsLogger) Logf(classification logging.Classification, format string, v ...interface{}) {
	switch classification {
	case logging.Warn:
		l.logger.Warningf(format, v)
	case logging.Debug:
		l.logger.Debugf(format, v)
	default:
		l.logger.Tracef(format, v)
	}
}

// NewS3Client returns a new s3Caller client for accessing the object store.
func NewS3Client(apiConn api.Connection, logger Logger) (objectstore.Session, error) {
	// We use api.Connection address because we assume this address is
	// correct and reachable.
	currentAPIAddress := apiConn.Addr()
	if currentAPIAddress == "" {
		return nil, errors.New("API address not available for S3 client")
	}

	apiHTTPClient, err := apiConn.HTTPClient()
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve http client from the api connection")
	}
	awsHTTPDoer := &awsHTTPDoer{
		client: apiHTTPClient,
	}
	awsLogger := &awsLogger{
		logger: logger,
	}

	httpsAPIAddress := ensureHTTPS(currentAPIAddress)

	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithLogger(awsLogger),
		config.WithHTTPClient(awsHTTPDoer),
		config.WithEndpointResolver(&awsEndpointResolver{endpoint: httpsAPIAddress}),
		// Standard retryer retries 3 times with 20s backoff time by
		// default.
		config.WithRetryer(func() aws.Retryer { return retry.NewStandard() }),
		// The anonymous credentials are needed by the aws sdk to
		// perform anonymous s3 access.
		config.WithCredentialsProvider(aws.AnonymousCredentials{}),
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot load default config for s3 client")
	}

	return &objectsClient{
		client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true
		}),
		logger: logger,
	}, nil
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
