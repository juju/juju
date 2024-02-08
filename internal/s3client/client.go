// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/logging"
	transporthttp "github.com/aws/smithy-go/transport/http"
	"github.com/juju/errors"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
	Tracef(message string, args ...any)

	IsTraceEnabled() bool
}

// HTTPClient represents the http client used to access the object store.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// CredentialsKind represents the kind of credentials used to access the object
// store.
type CredentialsKind string

const (
	// AnonymousCredentialsKind represents anonymous credentials.
	AnonymousCredentialsKind CredentialsKind = "anonymous"
	// StaticCredentialsKind represents static credentials.
	StaticCredentialsKind CredentialsKind = "static"
)

// Credentials represents the credentials used to access the object store.
type Credentials interface {
	Kind() CredentialsKind
}

// AnonymousCredentials represents anonymous credentials.
type AnonymousCredentials struct {
	Credentials
}

// Kind returns the kind of credentials.
func (AnonymousCredentials) Kind() CredentialsKind {
	return AnonymousCredentialsKind
}

// S3Client is a Juju shim around the AWS S3 client,
// which Juju uses to drive its object store requirements.
// StaticCredentials represents static credentials.
type StaticCredentials struct {
	Key     string
	Secret  string
	Session string
}

// Kind returns the kind of credentials.
func (StaticCredentials) Kind() CredentialsKind {
	return StaticCredentialsKind
}

// objectsClient is a Juju shim around the AWS S3 client,
// which Juju uses to drive it's object store requirents
type S3Client struct {
	logger Logger
	client *s3.Client
}

// NewS3Client returns a new s3Caller client for accessing the object store.
func NewS3Client(endpoint string, httpClient HTTPClient, credentials Credentials, logger Logger) (*S3Client, error) {
	credentialsProvider, err := getCredentialsProvider(credentials)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get credentials provider")
	}

	awsLogger := &awsLogger{
		logger: logger,
	}

	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithLogger(awsLogger),
		config.WithHTTPClient(httpClient),
		config.WithEndpointResolverWithOptions(&awsEndpointResolver{endpoint: endpoint}),
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
		config.WithCredentialsProvider(credentialsProvider),
	)
	if err != nil {
		return nil, errors.Annotate(err, "cannot load default config for s3 client")
	}

	return &S3Client{
		client: s3.NewFromConfig(cfg, func(o *s3.Options) {
			o.UsePathStyle = true
		}),
		logger: logger,
	}, nil
}

// HeadObject checks if an object exists in the object store based on the bucket
// name and object name.
// Returns nil if the object exists, or an error if it does not.
func (c *S3Client) HeadObject(ctx context.Context, bucketName, objectName string) error {
	c.logger.Tracef("checking if bucket %s object %s exists in s3 storage", bucketName, objectName)

	_, err := c.client.HeadObject(ctx,
		&s3.HeadObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectName),
		})
	if err != nil {
		if err := handleError(err); err != nil {
			return errors.Trace(err)
		}
		return errors.Annotatef(err, "checking if object %s exists on bucket %s using S3 client", objectName, bucketName)
	}

	return nil
}

// GetObject gets an object from the object store based on the bucket name and
// object name.
func (c *S3Client) GetObject(ctx context.Context, bucketName, objectName string) (io.ReadCloser, int64, string, error) {
	c.logger.Tracef("getting bucket %s object %s from s3 storage", bucketName, objectName)

	obj, err := c.client.GetObject(ctx,
		&s3.GetObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectName),
		})
	if err != nil {
		if err := handleError(err); err != nil {
			return nil, -1, "", errors.Trace(err)
		}
		return nil, -1, "", errors.Annotatef(err, "getting object %s on bucket %s using S3 client", objectName, bucketName)
	}
	var size int64
	if obj.ContentLength != nil {
		size = *obj.ContentLength
	}
	var hash string
	if obj.ChecksumSHA256 != nil {
		hash = *obj.ChecksumSHA256
	}
	return obj.Body, size, hash, nil
}

// ListObjects returns a list of objects in the specified bucket.
func (c *S3Client) ListObjects(ctx context.Context, bucketName string) ([]string, error) {
	c.logger.Tracef("listing objects in bucket %s from s3 storage", bucketName)

	objs, err := c.client.ListObjectsV2(ctx,
		&s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
		})
	if err != nil {
		if err := handleError(err); err != nil {
			return nil, errors.Trace(err)
		}
		return nil, errors.Annotatef(err, "listing objects on bucket %s using S3 client", bucketName)
	}

	var objects []string
	for _, obj := range objs.Contents {
		if obj.Key == nil {
			continue
		}
		objects = append(objects, *obj.Key)
	}
	return objects, nil
}

const (
	// retentionLockDate is the date used to set the retention lock on the
	// object. After that expires, the object can be deleted without
	// confirmation.
	retentionLockDate = 20 * 365 * 24 * time.Hour
)

// PutObject puts an object into the object store based on the bucket name and
// object name.
func (c *S3Client) PutObject(ctx context.Context, bucketName, objectName string, body io.Reader, hash string) error {
	c.logger.Tracef("putting bucket %s object %s to s3 storage", bucketName, objectName)

	obj, err := c.client.PutObject(ctx,
		&s3.PutObjectInput{
			Bucket:            aws.String(bucketName),
			Key:               aws.String(objectName),
			Body:              body,
			ChecksumAlgorithm: types.ChecksumAlgorithmSha256,

			// Prevent the object from being deleted for 10 years.
			// The lock is only here to prevent deletion when in the console.
			ObjectLockMode:            types.ObjectLockModeGovernance,
			ObjectLockLegalHoldStatus: types.ObjectLockLegalHoldStatusOn,
			ObjectLockRetainUntilDate: aws.Time(time.Now().Add(retentionLockDate)),
		})
	if err != nil {
		if err := handleError(err); err != nil {
			return errors.Trace(err)
		}
		return errors.Annotatef(err, "putting object %s on bucket %s using S3 client", objectName, bucketName)
	}
	if hash != "" && obj.ChecksumSHA256 != nil && hash != *obj.ChecksumSHA256 {
		return errors.Errorf("hash mismatch, expected %q got %q", hash, *obj.ChecksumSHA256)
	}
	return nil
}

// DeleteObject deletes an object from the object store based on the bucket name
// and object name.
func (c *S3Client) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	c.logger.Tracef("deleting bucket %s object %s from s3 storage", bucketName, objectName)

	_, err := c.client.DeleteObject(ctx,
		&s3.DeleteObjectInput{
			Bucket: aws.String(bucketName),
			Key:    aws.String(objectName),

			// We know we want to delete it, so we can bypass the retention.
			BypassGovernanceRetention: aws.Bool(true),
		})
	if err != nil {
		if err := handleError(err); err != nil {
			return errors.Trace(err)
		}
		return errors.Annotatef(err, "deleting object %s on bucket %s using S3 client", objectName, bucketName)
	}
	return nil
}

// CreateBucket creates a bucket in the object store based on the bucket name.
func (c *S3Client) CreateBucket(ctx context.Context, bucketName string) error {
	c.logger.Tracef("creating bucket %s in s3 storage", bucketName)

	_, err := c.client.CreateBucket(ctx,
		&s3.CreateBucketInput{
			Bucket:                     aws.String(bucketName),
			ObjectLockEnabledForBucket: aws.Bool(true),
		})
	if err != nil {
		if err := handleError(err); err != nil {
			return errors.Trace(err)
		}
		return errors.Annotatef(err, "unable to create bucket %s using S3 client", bucketName)
	}
	return nil
}

// forbiddenErrorCodes is a list of error codes that are returned when the
// credentials are invalid.
// https://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html#ErrorCodeList
var forbiddenErrorCodes = map[string]struct{}{
	"AccessDenied":          {},
	"InvalidAccessKeyId":    {},
	"InvalidSecurity":       {},
	"SignatureDoesNotMatch": {},
}

var alreadyExistCodes = map[string]struct{}{
	"BucketAlreadyExists":     {},
	"BucketAlreadyOwnedByYou": {},
}

var notFoundCodes = map[string]struct{}{
	"NoSuchBucket": {},
	"NoSuchKey":    {},
}

func handleError(err error) error {
	if err == nil {
		return nil
	}

	// Attempt to look up the error code and return a more specific error type.
	var ae smithy.APIError
	if errors.As(err, &ae) {
		errorCode := ae.ErrorCode()
		if _, ok := notFoundCodes[errorCode]; ok {
			return errors.NewNotFound(err, ae.ErrorMessage())
		}
		if _, ok := forbiddenErrorCodes[errorCode]; ok {
			return errors.NewForbidden(err, ae.ErrorMessage())
		}
		if _, ok := alreadyExistCodes[errorCode]; ok {
			return errors.NewAlreadyExists(err, ae.ErrorMessage())
		}
	}

	// If the error is a transport error, we can extract the HTTP status code
	// and use it to determine the error type.
	var oe *smithy.OperationError
	if errors.As(err, &oe) {
		var te *transporthttp.ResponseError
		if errors.As(oe.Err, &te) {
			statusCode := te.HTTPStatusCode()
			if statusCode == http.StatusForbidden {
				return errors.NewForbidden(err, fmt.Sprintf("http status %d", statusCode))
			}
			if statusCode == http.StatusNotFound {
				return errors.NewNotFound(err, fmt.Sprintf("http status %d", statusCode))
			}
		}
	}

	return errors.Trace(err)
}

type awsEndpointResolver struct {
	endpoint string
}

// ResolveEndpoint returns the endpoint for the given service and region.
func (a *awsEndpointResolver) ResolveEndpoint(_, _ string, options ...any) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL: a.endpoint,
	}, nil
}

type awsLogger struct {
	logger Logger
}

func (l *awsLogger) Logf(classification logging.Classification, format string, v ...any) {
	switch classification {
	case logging.Warn:
		l.logger.Warningf(format, v)
	case logging.Debug:
		l.logger.Debugf(format, v)
	default:
		l.logger.Tracef(format, v)
	}
}

func getCredentialsProvider(creds Credentials) (aws.CredentialsProvider, error) {
	switch creds.Kind() {
	case AnonymousCredentialsKind:
		return aws.AnonymousCredentials{}, nil
	case StaticCredentialsKind:
		s := creds.(StaticCredentials)
		return credentials.NewStaticCredentialsProvider(s.Key, s.Secret, s.Session), nil
	default:
		return nil, errors.Errorf("unknown credentials kind %q", creds.Kind())
	}
}

type unlimitedRateLimiter struct{}

func (unlimitedRateLimiter) AddTokens(uint) error { return nil }
func (unlimitedRateLimiter) GetToken(context.Context, uint) (func() error, error) {
	return noOpToken, nil
}
func noOpToken() error { return nil }
