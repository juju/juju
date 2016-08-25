// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/amz.v3/aws"
	"gopkg.in/amz.v3/ec2"
	"gopkg.in/amz.v3/s3"

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

// Open is specified in the EnvironProvider interface.
func (p environProvider) Open(args environs.OpenParams) (environs.Environ, error) {
	logger.Infof("opening model %q", args.Config.Name())

	e := new(environ)
	e.cloud = args.Cloud
	e.name = args.Config.Name()

	var err error
	e.ec2, e.s3, err = awsClients(args.Cloud)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if err := e.SetConfig(args.Config); err != nil {
		return nil, errors.Trace(err)
	}
	return e, nil
}

func awsClients(cloud environs.CloudSpec) (*ec2.EC2, *s3.S3, error) {
	if err := validateCloudSpec(cloud); err != nil {
		return nil, nil, errors.Annotate(err, "validating cloud spec")
	}

	credentialAttrs := cloud.Credential.Attributes()
	accessKey := credentialAttrs["access-key"]
	secretKey := credentialAttrs["secret-key"]
	auth := aws.Auth{
		AccessKey: accessKey,
		SecretKey: secretKey,
	}

	// TODO(axw) define region in terms of EC2 and S3 endpoints.
	region := aws.Regions[cloud.Region]
	signer := aws.SignV4Factory(region.Name, "ec2")
	return ec2.New(auth, region, signer), s3.New(auth, region), nil
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
	if _, ok := aws.Regions[c.Region]; !ok {
		return errors.NotValidf("region name %q", c.Region)
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
	ec2Region, ok := allRegions[region]
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
