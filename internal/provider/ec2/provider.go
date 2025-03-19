// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/juju/errors"
	"github.com/juju/jsonschema"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/provider/common"
)

var logger = internallogger.GetLogger("juju.provider.ec2")

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
func (p environProvider) Open(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
	logger.Debugf(ctx, "opening model %q", args.Config.Name())

	e := newEnviron(args.Config.Name(), args.ControllerUUID, invalidator)

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
func (p environProvider) Ping(_ context.Context, _ string) error {
	return errors.NotImplementedf("Ping")
}

// ValidateCloud is specified in the EnvironProvider interface.
func (environProvider) ValidateCloud(ctx context.Context, spec environscloudspec.CloudSpec) error {
	return errors.Annotate(validateCloudSpec(spec), "validating cloud spec")
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
var verifyCredentials = func(ctx context.Context, invalidator environs.CredentialInvalidator, e Client) error {
	_, err := e.DescribeAccountAttributes(ctx, nil)
	if err == nil {
		return nil
	}

	converted := convertAuthorizationError(err)
	if invalidator == nil {
		return converted
	}

	reason := environs.CredentialInvalidReason(converted.Error())
	return invalidator.InvalidateCredentials(ctx, reason)
}

// isAuthorizationError returns true if the error is an authorization error.
// This is used to determine if the error is related to the credentials used
// to authenticate with the cloud.
func isAuthorizationError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, common.ErrorCredentialNotValid) {
		return true
	}

	switch ec2ErrCode(err) {
	case "AuthFailure", "InvalidClientTokenId", "MissingAuthenticationToken", "Blocked",
		"CustomerKeyHasBeenRevoked", "PendingVerification", "SignatureDoesNotMatch":
		return true
	default:
		return false
	}
}

func convertAuthorizationError(err error) error {
	// If the error is nil, there's nothing to convert.
	if err == nil {
		return nil
	}

	// Don't convert an error that's already been converted.
	if errors.Is(err, common.ErrorCredentialNotValid) {
		return err
	}

	// EC2 error codes are from https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html.
	switch ec2ErrCode(err) {
	case "AuthFailure":
		return fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err))
	case "InvalidClientTokenId":
		return fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err))
	case "MissingAuthenticationToken":
		return fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err))
	case "Blocked":
		return fmt.Errorf("\nYour Amazon account is currently blocked.: %w",
			common.CredentialNotValidError(err))

	case "CustomerKeyHasBeenRevoked":
		return fmt.Errorf("\nYour Amazon keys have been revoked.: %w",
			common.CredentialNotValidError(err))

	case "PendingVerification":
		return fmt.Errorf("\nYour account is pending verification by Amazon.: %w",
			common.CredentialNotValidError(err))

	case "SignatureDoesNotMatch":
		return fmt.Errorf(badKeysFormat, common.CredentialNotValidError(err))
	default:
		// This error is unrelated to access keys, account or credentials...
		return err
	}
}

type credentialInvalidator struct {
	invalidator common.CredentialInvalidator
}

// newCredentialInvalidator returns a new credentialInvalidator that provides
// a user-friendly message when the error is related to the credentials.
func newCredentialInvalidator(invalidator common.CredentialInvalidator) credentialInvalidator {
	return credentialInvalidator{
		invalidator: invalidator,
	}
}

// InvalidateCredentials invalidates the credentials.
// This updates the error to include a user-friendly message if the error is
// related to the credentials.
func (c credentialInvalidator) InvalidateCredentials(ctx context.Context, reason environs.CredentialInvalidReason) error {
	return c.invalidator.InvalidateCredentials(ctx, reason)
}

// HandleCredentialError determines if a given error relates to an invalid
// credential. If it is, the credential is invalidated and the error is
// update to include a user-friendly message.
func (c credentialInvalidator) HandleCredentialError(ctx context.Context, err error) error {
	return c.invalidator.HandleCredentialError(ctx, convertAuthorizationError(err))
}

// MaybeInvalidateCredentialError determines if a given error relates to an invalid
// credential. If it is, the credential is invalidated and the error is
// update to include a user-friendly message.
func (c credentialInvalidator) MaybeInvalidateCredentialError(ctx context.Context, err error) (bool, error) {
	return c.invalidator.MaybeInvalidateCredentialError(ctx, convertAuthorizationError(err))
}
