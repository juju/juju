// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	stdcontext "context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo/v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/provider/common"
)

var logger = loggo.GetLogger("juju.provider.ec2")

type environProvider struct {
	environProviderCloud
	environProviderCredentials
}

var providerInstance environProvider

// Version is part of the EnvironProvider interface.
func (environProvider) Version() int {
	return 0
}

// Open is specified in the EnvironProvider interface.
func (p environProvider) Open(ctx stdcontext.Context, args environs.OpenParams) (environs.Environ, error) {
	logger.Debugf("opening model %q", args.Config.Name())

	e := newEnviron()
	e.name = args.Config.Name()
	e.controllerUUID = args.ControllerUUID

	namespace, err := instance.NewNamespace(args.Config.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	e.namespace = namespace

	if err := e.SetCloudSpec(ctx, args.Cloud); err != nil {
		return nil, err
	}

	if err := e.SetConfig(ctx, args.Config); err != nil {
		return nil, errors.Trace(err)
	}
	return e, nil
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(ctx envcontext.ProviderCallContext, endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p environProvider) PrepareConfig(ctx context.Context, args environs.PrepareConfigParams) (*config.Config, error) {
	if err := validateCloudSpec(args.Cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}
	// Set the default block-storage source.
	attrs := make(map[string]interface{})
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs[config.StorageDefaultBlockSourceKey] = EBS_ProviderType
	}
	if len(attrs) == 0 {
		return args.Config, nil
	}
	return args.Config.Apply(attrs)
}

func validateCloudSpec(c environscloudspec.CloudSpec) error {
	if err := c.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := c.Credential.AuthType(); authType != cloud.AccessKeyAuthType &&
		authType != cloud.InstanceRoleAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}

// Validate is specified in the EnvironProvider interface.
func (environProvider) Validate(ctx context.Context, cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(ctx, cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid EC2 provider config: %v", err)
	}
	return newEcfg.Apply(newEcfg.attrs)
}

// AgentMetadataLookupParams returns parameters which are used to query agent metadata to
// find matching image information.
func (p environProvider) AgentMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	return p.metadataLookupParams(region)
}

// ImageMetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p environProvider) ImageMetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	return p.metadataLookupParams(region)
}

func (p environProvider) metadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, fmt.Errorf("region must be specified")
	}
	resolver := ec2.NewDefaultEndpointResolver()
	ep, err := resolver.ResolveEndpoint("us-east-1", ec2.EndpointResolverOptions{})
	if err != nil {
		return nil, errors.Annotatef(err, "unknown region %q", region)
	}
	return &simplestreams.MetadataLookupParams{
		Region:   region,
		Endpoint: ep.URL,
	}, nil
}

const badKeysFormat = `
The provided credentials could not be validated and 
may not be authorized to carry out the request.
Ensure that your account is authorized to use the Amazon EC2 service and 
that you are using the correct access keys. 
These keys are obtained via the "Security Credentials"
page in the AWS console: %w
`

// verifyCredentials issues a cheap, non-modifying/idempotent request to EC2 to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e Client, ctx envcontext.ProviderCallContext) error {
	_, err := e.DescribeAccountAttributes(ctx, nil)
	return maybeConvertCredentialError(err, ctx)
}

// maybeConvertCredentialError examines the error received from the provider.
// Authentication related errors conform to common.ErrorCredentialNotValid.
// Authorisation related errors are annotated with an additional
// user-friendly explanation.
// All other errors are returned un-wrapped and not annotated.
var maybeConvertCredentialError = func(err error, ctx envcontext.ProviderCallContext) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, common.ErrorCredentialNotValid) {
		return err
	}

	convert := func(converted error) error {
		callbackErr := ctx.InvalidateCredential(converted.Error())
		if callbackErr != nil {
			// We want to proceed with the actual processing but still keep a log of a problem.
			logger.Infof("callback to invalidate model credential failed with %v", converted)
		}
		return converted
	}

	// EC2 error codes are from https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html.
	switch ec2ErrCode(err) {
	case "AuthFailure":
		return convert(fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err)))
	case "InvalidClientTokenId":
		return convert(fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err)))
	case "MissingAuthenticationToken":
		return convert(fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err)))
	case "Blocked":
		return convert(
			fmt.Errorf("\nYour Amazon account is currently blocked.: %w",
				common.CredentialNotValidError(err)),
		)
	case "CustomerKeyHasBeenRevoked":
		return convert(
			fmt.Errorf("\nYour Amazon keys have been revoked.: %w",
				common.CredentialNotValidError(err)),
		)
	case "PendingVerification":
		return convert(
			fmt.Errorf("\nYour account is pending verification by Amazon.: %w",
				common.CredentialNotValidError(err)),
		)
	case "SignatureDoesNotMatch":
		return convert(fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err)))
	default:
		// This error is unrelated to access keys, account or credentials...
		return err
	}
}
