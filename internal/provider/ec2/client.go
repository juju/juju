// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/ec2rolecreds"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/smithy-go/logging"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/cloudspec"
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

type ClientFunc = func(context.Context, cloudspec.CloudSpec, ...ClientOption) (Client, error)

// Client defines the subset of *ec2.Client methods that we currently use.
type Client interface {
	// STOP!!
	// Are you about to add a new function to this interface?
	// If so please make sure you update Juju permission policy on discourse
	// here https://discourse.charmhub.io/t/juju-aws-permissions/5307
	// We must keep this policy inline with our usage for operators that are
	// using very strict permissions for Juju.
	//
	// You must also update the controllerRolePolicy document found in
	// iam_docs.go.
	AssociateIamInstanceProfile(context.Context, *ec2.AssociateIamInstanceProfileInput, ...func(*ec2.Options)) (*ec2.AssociateIamInstanceProfileOutput, error)
	DescribeIamInstanceProfileAssociations(context.Context, *ec2.DescribeIamInstanceProfileAssociationsInput, ...func(*ec2.Options)) (*ec2.DescribeIamInstanceProfileAssociationsOutput, error)
	DescribeInstances(context.Context, *ec2.DescribeInstancesInput, ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeInstanceTypes(context.Context, *ec2.DescribeInstanceTypesInput, ...func(*ec2.Options)) (*ec2.DescribeInstanceTypesOutput, error)
	DescribeSpotPriceHistory(context.Context, *ec2.DescribeSpotPriceHistoryInput, ...func(*ec2.Options)) (*ec2.DescribeSpotPriceHistoryOutput, error)

	DescribeAvailabilityZones(context.Context, *ec2.DescribeAvailabilityZonesInput, ...func(*ec2.Options)) (*ec2.DescribeAvailabilityZonesOutput, error)
	RunInstances(context.Context, *ec2.RunInstancesInput, ...func(*ec2.Options)) (*ec2.RunInstancesOutput, error)
	TerminateInstances(context.Context, *ec2.TerminateInstancesInput, ...func(*ec2.Options)) (*ec2.TerminateInstancesOutput, error)

	DescribeAccountAttributes(context.Context, *ec2.DescribeAccountAttributesInput, ...func(*ec2.Options)) (*ec2.DescribeAccountAttributesOutput, error)

	DescribeSecurityGroups(context.Context, *ec2.DescribeSecurityGroupsInput, ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
	CreateSecurityGroup(context.Context, *ec2.CreateSecurityGroupInput, ...func(*ec2.Options)) (*ec2.CreateSecurityGroupOutput, error)
	DeleteSecurityGroup(context.Context, *ec2.DeleteSecurityGroupInput, ...func(*ec2.Options)) (*ec2.DeleteSecurityGroupOutput, error)
	AuthorizeSecurityGroupIngress(context.Context, *ec2.AuthorizeSecurityGroupIngressInput, ...func(*ec2.Options)) (*ec2.AuthorizeSecurityGroupIngressOutput, error)
	RevokeSecurityGroupIngress(context.Context, *ec2.RevokeSecurityGroupIngressInput, ...func(*ec2.Options)) (*ec2.RevokeSecurityGroupIngressOutput, error)

	CreateTags(context.Context, *ec2.CreateTagsInput, ...func(*ec2.Options)) (*ec2.CreateTagsOutput, error)

	CreateVolume(context.Context, *ec2.CreateVolumeInput, ...func(*ec2.Options)) (*ec2.CreateVolumeOutput, error)
	AttachVolume(context.Context, *ec2.AttachVolumeInput, ...func(*ec2.Options)) (*ec2.AttachVolumeOutput, error)
	DetachVolume(context.Context, *ec2.DetachVolumeInput, ...func(*ec2.Options)) (*ec2.DetachVolumeOutput, error)
	DeleteVolume(context.Context, *ec2.DeleteVolumeInput, ...func(*ec2.Options)) (*ec2.DeleteVolumeOutput, error)
	DescribeVolumes(context.Context, *ec2.DescribeVolumesInput, ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)

	DescribeNetworkInterfaces(context.Context, *ec2.DescribeNetworkInterfacesInput, ...func(*ec2.Options)) (*ec2.DescribeNetworkInterfacesOutput, error)
	DescribeSubnets(context.Context, *ec2.DescribeSubnetsInput, ...func(*ec2.Options)) (*ec2.DescribeSubnetsOutput, error)
	DescribeVpcs(context.Context, *ec2.DescribeVpcsInput, ...func(*ec2.Options)) (*ec2.DescribeVpcsOutput, error)
	DescribeInternetGateways(context.Context, *ec2.DescribeInternetGatewaysInput, ...func(*ec2.Options)) (*ec2.DescribeInternetGatewaysOutput, error)
	DescribeRouteTables(context.Context, *ec2.DescribeRouteTablesInput, ...func(*ec2.Options)) (*ec2.DescribeRouteTablesOutput, error)
}

type awsLogger struct {
	cfg *config.Config
}

func (l awsLogger) Write(p []byte) (n int, err error) {
	logger.Tracef("awsLogger %p: %s", l.cfg, p)
	return len(p), nil
}

// clientFunc returns a ec2 client with the given credentials.
func clientFunc(ctx context.Context, spec cloudspec.CloudSpec, clientOptions ...ClientOption) (Client, error) {
	cfg, err := configFromCloudSpec(ctx, spec, clientOptions...)
	if err != nil {
		return nil, errors.Annotate(err, "building aws config from cloudspec")
	}
	return ec2.NewFromConfig(cfg), nil
}

func configFromCloudSpec(
	ctx context.Context,
	spec cloudspec.CloudSpec,
	clientOptions ...ClientOption,
) (aws.Config, error) {
	var credentialProvider aws.CredentialsProvider = ec2rolecreds.New()
	if spec.Credential.AuthType() == cloud.AccessKeyAuthType {
		credentialAttrs := spec.Credential.Attributes()
		credentialProvider = credentials.NewStaticCredentialsProvider(
			credentialAttrs["access-key"],
			credentialAttrs["secret-key"],
			"",
		)
	}

	// The default retry attempts and max backoff are a little too low
	// on a busy system, especially running CI tests.
	retrier := retry.AddWithMaxAttempts(retry.NewStandard(), 10)
	retrier = retry.AddWithMaxBackoffDelay(retrier, time.Minute)
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(spec.Region),
		config.WithRetryer(func() aws.Retryer {
			return retrier
		}),
		config.WithCredentialsProvider(credentialProvider),
	)
	if err != nil {
		return aws.Config{}, errors.Trace(err)
	}

	// Enable request and response logging, but only if TRACE is enabled (as
	// they're probably fairly expensive to produce).
	if logger.IsTraceEnabled() {
		cfg.ClientLogMode = aws.LogRequest | aws.LogResponse | aws.LogRetries
		cfg.Logger = logging.NewStandardLogger(&awsLogger{})
	}
	opts := newOptions()
	for _, option := range clientOptions {
		option(opts)
	}
	cfg.HTTPClient = opts.httpClient

	return cfg, nil
}
