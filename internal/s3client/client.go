// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

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

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...interface{})
	Warningf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Debugf(message string, args ...interface{})
	Tracef(message string, args ...interface{})
}

// S3Client represents the S3 client methods required by objectClient
type S3Client interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// Session represents the interface objectClient exports to interact with S3
type Session interface {
	GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, error)
}

// objectsClient is a Juju shim around the AWS S3 client,
// which Juju uses to drive it's object store requirents
type objectsClient struct {
	logger Logger
	client S3Client
}

// GetObject retrieves an object from an S3 object store. Returns a
// stream containing the object's content
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

func (a *awsEndpointResolver) ResolveEndpoint(_, _ string) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL: a.endpoint,
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

type unlimitedRateLimiter struct{}

func (unlimitedRateLimiter) AddTokens(uint) error { return nil }
func (unlimitedRateLimiter) GetToken(context.Context, uint) (func() error, error) {
	return noOpToken, nil
}
func noOpToken() error { return nil }

// NewS3Client creates a generic S3 client which Juju should use to
// drive it's object store requirements
func NewS3Client(apiConn api.Connection, logger Logger) (Session, error) {
	apiHTTPClient, err := apiConn.RootHTTPClient()
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
		config.WithEndpointResolver(&awsEndpointResolver{endpoint: apiHTTPClient.BaseURL}),
		// Standard retryer with custom max attempts. Will retry at most
		// 10 times with 20s backoff time.
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(
				func(o *retry.StandardOptions) {
					o.MaxAttempts = 10
					o.RateLimiter = unlimitedRateLimiter{}
				},
			)
		}),
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
