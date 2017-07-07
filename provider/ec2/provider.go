// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/jsonschema"
	"github.com/juju/loggo"
	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
)

var logger = loggo.GetLogger("juju.provider.ec2")

type environProvider struct {
	environProviderCredentials
}

var providerInstance environProvider

// Version is part of the EnvironProvider interface.
func (environProvider) Version() int {
	return 0
}

// Open is specified in the EnvironProvider interface.
func (p environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Infof("opening model %q", args.Config.Name())

	e := new(environ)
	e.cloud = args.Cloud
	e.name = args.Config.Name()

	// The endpoints in public-clouds.yaml from 2.0-rc2
	// and before were wrong, so we use whatever is defined
	// in goamz/aws if available.
	if isBrokenCloud(e.cloud) {
		if region, ok := aws.Regions[e.cloud.Region]; ok {
			e.cloud.Endpoint = region.EC2Endpoint
		}
	}

	var err error
	e.ec2, err = awsClient(e.cloud)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := e.SetConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}
	return e, nil
}

// isBrokenCloud reports whether the given CloudSpec is from an old,
// broken version of public-clouds.yaml.
func isBrokenCloud(cloud environs.CloudSpec) bool {
	// The public-clouds.yaml from 2.0-rc2 and before was
	// complete nonsense for general regions and for
	// govcloud. The cn-north-1 region has a trailing slash,
	// which we don't want as it means we won't match the
	// simplestreams data.
	switch cloud.Region {
	case "us-east-1", "us-west-1", "us-west-2", "eu-west-1",
		"eu-central-1", "ap-southeast-1", "ap-southeast-2",
		"ap-northeast-1", "ap-northeast-2", "sa-east-1":
		return cloud.Endpoint == fmt.Sprintf("https://%s.aws.amazon.com/v1.2/", cloud.Region)
	case "cn-north-1":
		return strings.HasSuffix(cloud.Endpoint, "/")
	case "us-gov-west-1":
		return cloud.Endpoint == "https://ec2.us-gov-west-1.amazonaws-govcloud.com"
	}
	return false
}

func awsClient(cloud environs.CloudSpec) (*ec2.EC2, error) {
	if err := validateCloudSpec(cloud); err != nil {
		return nil, errors.Annotate(err, "validating cloud spec")
	}

	credentialAttrs := cloud.Credential.Attributes()
	accessKey := credentialAttrs["access-key"]
	secretKey := credentialAttrs["secret-key"]
	auth := aws.Auth{
		AccessKey: accessKey,
		SecretKey: secretKey,
	}

	region := aws.Region{
		Name:        cloud.Region,
		EC2Endpoint: cloud.Endpoint,
	}
	signer := aws.SignV4Factory(cloud.Region, "ec2")
	return ec2.New(auth, region, signer), nil
}

// CloudSchema returns the schema used to validate input for add-cloud.  Since
// this provider does not support custom clouds, this always returns nil.
func (p environProvider) CloudSchema() *jsonschema.Schema {
	return nil
}

// Ping tests the connection to the cloud, to verify the endpoint is valid.
func (p environProvider) Ping(endpoint string) error {
	return errors.NotImplementedf("Ping")
}

// PrepareConfig is specified in the EnvironProvider interface.
func (p environProvider) PrepareConfig(args environs.PrepareConfigParams) (*config.Config, error) {
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

func validateCloudSpec(c environs.CloudSpec) error {
	if err := c.Validate(); err != nil {
		return errors.Trace(err)
	}
	if c.Credential == nil {
		return errors.NotValidf("missing credential")
	}
	if authType := c.Credential.AuthType(); authType != cloud.AccessKeyAuthType {
		return errors.NotSupportedf("%q auth-type", authType)
	}
	return nil
}

// Validate is specified in the EnvironProvider interface.
func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid EC2 provider config: %v", err)
	}
	return newEcfg.Apply(newEcfg.attrs)
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p environProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		return nil, fmt.Errorf("region must be specified")
	}
	ec2Region, ok := aws.Regions[region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q", region)
	}
	return &simplestreams.MetadataLookupParams{
		Region:   region,
		Endpoint: ec2Region.EC2Endpoint,
	}, nil
}

const badAccessKey = `
Please ensure the Access Key ID you have specified is correct.
You can obtain the Access Key ID via the "Security Credentials"
page in the AWS console.`

const badSecretKey = `
Please ensure the Secret Access Key you have specified is correct.
You can obtain the Secret Access Key via the "Security Credentials"
page in the AWS console.`

// verifyCredentials issues a cheap, non-modifying/idempotent request to EC2 to
// verify the configured credentials. If verification fails, a user-friendly
// error will be returned, and the original error will be logged at debug
// level.
var verifyCredentials = func(e *environ) error {
	_, err := e.ec2.AccountAttributes()
	if err != nil {
		logger.Debugf("ec2 request failed: %v", err)
		if err, ok := err.(*ec2.Error); ok {
			switch err.Code {
			case "AuthFailure":
				return errors.New("authentication failed.\n" + badAccessKey)
			case "SignatureDoesNotMatch":
				return errors.New("authentication failed.\n" + badSecretKey)
			default:
				return err
			}
		}
		return err
	}
	return nil
}
