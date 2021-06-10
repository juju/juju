// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/client"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

// ClientOption to be passed into the transport construction to customize the
// default transport.
type ClientOption func(*clientOptions)

type clientOptions struct {
	httpClient *http.Client
}

// WithHTTPClient allows to define the http.Client to use.
func WithHTTPClient(value *http.Client) ClientOption {
	return func(opt *clientOptions) {
		opt.httpClient = value
	}
}

// Create a clientOptions instance with default values.
func newOptions() *clientOptions {
	defaultCopy := *http.DefaultClient
	return &clientOptions{
		httpClient: &defaultCopy,
	}
}

type ClientFunc = func(string, string, string, ...ClientOption) Client

// The subset of *ec2.EC2 methods that we currently use.
type Client interface {
	DescribeAvailabilityZones(*ec2.DescribeAvailabilityZonesInput) (*ec2.DescribeAvailabilityZonesOutput, error)
	DescribeInstances(*ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
	DescribeInstanceTypeOfferings(*ec2.DescribeInstanceTypeOfferingsInput) (*ec2.DescribeInstanceTypeOfferingsOutput, error)
	DescribeInstanceTypes(*ec2.DescribeInstanceTypesInput) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeSpotPriceHistory(*ec2.DescribeSpotPriceHistoryInput) (*ec2.DescribeSpotPriceHistoryOutput, error)
}

// EC2Session returns a session with the given credentials.
func clientFunc(region, accessKey, secretKey string, clientOptions ...ClientOption) Client {
	panic("Z")
	opts := newOptions()
	for _, option := range clientOptions {
		option(opts)
	}

	sess := session.Must(session.NewSession())
	config := &aws.Config{
		HTTPClient: opts.httpClient,
		Retryer: client.DefaultRetryer{ // these roughly match retry params in gopkg.in/amz.v3/ec2/ec2.go:EC2.query
			NumMaxRetries:    10,
			MinRetryDelay:    time.Second,
			MinThrottleDelay: time.Second,
			MaxRetryDelay:    time.Minute,
			MaxThrottleDelay: time.Minute,
		},
		Region: aws.String(region),
		Credentials: credentials.NewStaticCredentialsFromCreds(credentials.Value{
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
		}),
	}

	// Enable request and response logging, but only if TRACE is enabled (as
	// they're probably fairly expensive to produce).
	if logger.IsTraceEnabled() {
		config.Logger = awsLogger{sess}
		config.LogLevel = aws.LogLevel(aws.LogDebug | aws.LogDebugWithRequestErrors | aws.LogDebugWithRequestRetries)
	}

	return ec2.New(sess, config)
}
