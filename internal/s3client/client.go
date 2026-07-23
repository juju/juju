// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package s3client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

	"github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

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

// Option represents an option for configuring the S3Client.
type Option func(*options)

type options struct {
	maxAttempts       int
	allowRateLimiting bool
	logger            logger.Logger
	region            string
}

// WithMaxAttempts is the option to set the maximum number of attempts for the
// S3 client.
func WithMaxAttempts(maxAttempts int) Option {
	return func(o *options) {
		o.maxAttempts = maxAttempts
	}
}

// WithRateLimiting is the option to enable rate limiting for the S3 client.
func WithRateLimiting(allowRateLimiting bool) Option {
	return func(o *options) {
		o.allowRateLimiting = allowRateLimiting
	}
}

// WithLogger is the option to set the logger for the S3 client.
func WithLogger(logger logger.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithRegion is the option to set the signing region for the S3 client.
// When empty (the default), the region is derived from the endpoint URL for
// common AWS forms. If derivation fails and static credentials are used, a
// placeholder region is used and a warning is logged; for anonymous
// credentials the region is left empty since signing is skipped.
func WithRegion(region string) Option {
	return func(o *options) {
		o.region = region
	}
}

func defaultOptions() *options {
	return &options{
		maxAttempts:       10,
		allowRateLimiting: false,
		logger:            internallogger.GetLogger("s3client"),
	}
}

// objectsClient is a Juju shim around the AWS S3 client,
// which Juju uses to drive it's object store requirents
type S3Client struct {
	logger logger.Logger
	client *s3.Client
}

// NewS3Client returns a new s3Caller client for accessing the object store.
func NewS3Client(endpoint string, httpClient HTTPClient, credentials Credentials, opts ...Option) (*S3Client, error) {
	o := defaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	credentialsProvider, err := getCredentialsProvider(credentials)
	if err != nil {
		return nil, errors.Annotate(err, "cannot get credentials provider")
	}

	awsLogger := &awsLogger{
		logger: o.logger.Child("aws.s3"),
	}

	region, err := resolveRegion(o.region, endpoint)
	if err != nil {
		if credentials.Kind() == StaticCredentialsKind {
			o.logger.Warningf(context.Background(),
				"could not determine S3 signing region from endpoint %q; "+
					"using placeholder %q. Set the object-store-s3-region "+
					"controller config key to specify the correct region.",
				endpoint, fallbackRegion)
			region = fallbackRegion
		}
		// Anonymous credentials skip SigV4 signing, so an empty region is fine.
	}

	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithLogger(awsLogger),
		config.WithHTTPClient(httpClient),
		config.WithRegion(region),
		config.WithEndpointResolverWithOptions(&awsEndpointResolver{endpoint: endpoint, region: region}),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(
				func(s3Options *retry.StandardOptions) {
					if o.maxAttempts > 0 {
						s3Options.MaxAttempts = o.maxAttempts
					}
					if !o.allowRateLimiting {
						s3Options.RateLimiter = unlimitedRateLimiter{}
					}
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
		logger: o.logger,
	}, nil
}

// ObjectExists checks if an object exists in the object store based on the bucket
// name and object name.
// Returns nil if the object exists, or an error if it does not.
func (c *S3Client) ObjectExists(ctx context.Context, bucketName, objectName string) error {
	c.logger.Tracef(ctx, "checking if bucket %s object %s exists in s3 storage", bucketName, objectName)

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
	c.logger.Tracef(ctx, "getting bucket %s object %s from s3 storage", bucketName, objectName)

	obj, err := c.client.GetObject(ctx,
		&s3.GetObjectInput{
			Bucket:       aws.String(bucketName),
			Key:          aws.String(objectName),
			ChecksumMode: types.ChecksumModeEnabled,
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

// ListObjects returns a list of objects in the specified bucket, optionally
// filtered by prefix.
func (c *S3Client) ListObjects(ctx context.Context, bucketName, prefix string) ([]string, error) {
	c.logger.Tracef(ctx, "listing objects in bucket %s with prefix %s from s3 storage", bucketName, prefix)

	objs, err := c.client.ListObjectsV2(ctx,
		&s3.ListObjectsV2Input{
			Bucket: aws.String(bucketName),
			Prefix: aws.String(prefix),
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
	c.logger.Tracef(ctx, "putting bucket %s object %s to s3 storage", bucketName, objectName)

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
	c.logger.Tracef(ctx, "deleting bucket %s object %s from s3 storage", bucketName, objectName)

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
	c.logger.Tracef(ctx, "creating bucket %s in s3 storage", bucketName)

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
	if ae, ok := errors.AsType[smithy.APIError](err); ok {
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
	if oe, ok := errors.AsType[*smithy.OperationError](err); ok {
		if te, ok := errors.AsType[*transporthttp.ResponseError](oe.Err); ok {
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

// awsGlobalEndpointRegion is the region the AWS global S3 endpoint
// (s3.amazonaws.com) maps to.
const awsGlobalEndpointRegion = "us-east-1"

// fallbackRegion is the placeholder SigV4 signing region used for static
// credentials when no region is set explicitly and none can be derived from
// the endpoint. It is deliberately not a real AWS region: S3-compatible
// stores that ignore the region (MinIO, Ceph) accept it, while real AWS
// requests fail loudly with this value visible in the error. Set
// object-store-s3-region to override.
const fallbackRegion = "juju-unknown-region"

// regionFromEndpoint attempts to extract an AWS region from a common S3
// endpoint URL. It recognises the following host forms:
//
//   - s3.amazonaws.com (global endpoint, returns us-east-1)
//   - s3.<region>.amazonaws.com
//   - s3-<region>.amazonaws.com
//   - <bucket>.s3.<region>.amazonaws.com (virtual-hosted style)
//   - <bucket>.s3-<region>.amazonaws.com (virtual-hosted style)
//
// Non-AWS hosts or unparseable endpoints return an empty string.
func regionFromEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return ""
	}
	host := u.Hostname()
	if !strings.HasSuffix(host, "amazonaws.com") {
		return ""
	}
	labels := strings.Split(host, ".")
	for i, label := range labels {
		if label == "s3" {
			if i+1 < len(labels) && labels[i+1] != "amazonaws" {
				return labels[i+1]
			}
			return awsGlobalEndpointRegion
		}
		if region, ok := strings.CutPrefix(label, "s3-"); ok {
			return region
		}
	}
	return ""
}

// resolveRegion determines the effective signing region following the
// precedence: explicit override, then derived from the endpoint. If neither
// yields a region, an error is returned.
func resolveRegion(override, endpoint string) (string, error) {
	if override != "" {
		return override, nil
	}
	if region := regionFromEndpoint(endpoint); region != "" {
		return region, nil
	}
	return "", errors.Errorf("region could not be derived from endpoint %q", endpoint)
}

type awsEndpointResolver struct {
	endpoint string
	region   string
}

// ResolveEndpoint returns the endpoint for the given service and region.
func (a *awsEndpointResolver) ResolveEndpoint(_, _ string, options ...any) (aws.Endpoint, error) {
	return aws.Endpoint{
		URL:           a.endpoint,
		SigningRegion: a.region,
	}, nil
}

type awsLogger struct {
	logger logger.Logger
}

func (l *awsLogger) Logf(classification logging.Classification, format string, v ...any) {
	switch classification {
	case logging.Warn:
		l.logger.Warningf(context.Background(), format, v...)
	case logging.Debug:
		l.logger.Debugf(context.Background(), format, v...)
	default:
		l.logger.Tracef(context.Background(), format, v...)
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
