// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3caller

import (
	"context"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/logging"
	"github.com/juju/errors"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
)

type Session interface {
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
}

type objectsClient struct {
	logger Logger
	client *s3.Client
}

func (c *objectsClient) GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error) {
	c.logger.Tracef("retrieving bucket %s object %s from s3 storage", bucketName, objectName)
	obj, err := c.client.GetObject(ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectName),
		})
	if err != nil {
		return nil, errors.Annotatef(err, "unable to get object %s on bucket %s using S3 client", objectName, bucketName)
	}
	return obj.Body, nil
}

type awsEndpointResolver struct {
	endpoint string
}

func (a *awsEndpointResolver) ResolveEndpoint(service, region string) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL: "https://" + a.endpoint,
	}, nil

}

type awsHTTPDoer struct {
	client *httprequest.Client
}

func (c *awsHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	var res *http.Response
	err := c.client.Do(context.Background(), req, &res)

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

func NewS3Client(apiConn api.Connection, logger Logger) (Session, error) {
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

	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithLogger(awsLogger),
		config.WithHTTPClient(awsHTTPDoer),
		config.WithEndpointResolver(&awsEndpointResolver{endpoint: currentAPIAddress}),
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
