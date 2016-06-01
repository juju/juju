// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/arch"
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

// RestrictedConfigAttributes is specified in the EnvironProvider interface.
func (p environProvider) RestrictedConfigAttributes() []string {
	// TODO(dimitern): Both of these shouldn't be restricted for hosted models.
	// See bug http://pad.lv/1580417 for more information.
	return []string{"region", "vpc-id-force"}
}

// PrepareForCreateEnvironment is specified in the EnvironProvider interface.
func (p environProvider) PrepareForCreateEnvironment(cfg *config.Config) (*config.Config, error) {
	e, err := p.Open(cfg)
	if err != nil {
		return nil, err
	}
	env := e.(*environ)

	apiClient, modelName, vpcID := env.ec2(), env.Name(), env.ecfg().vpcID()
	if err := validateModelVPC(apiClient, modelName, vpcID); err != nil {
		return nil, errors.Trace(err)
	}

	return cfg, nil
}

// Open is specified in the EnvironProvider interface.
func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening model %q", cfg.Name())
	e := new(environ)
	e.name = cfg.Name()
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	return e, nil
}

// BootstrapConfig is specified in the EnvironProvider interface.
func (p environProvider) BootstrapConfig(args environs.BootstrapConfigParams) (*config.Config, error) {
	// Add credentials to the configuration.
	attrs := map[string]interface{}{
		"region": args.CloudRegion,
		// TODO(axw) stop relying on hard-coded
		//           region endpoint information
		//           in the provider, and use
		//           args.CloudEndpoint here.
	}
	switch authType := args.Credentials.AuthType(); authType {
	case cloud.AccessKeyAuthType:
		credentialAttrs := args.Credentials.Attributes()
		attrs["access-key"] = credentialAttrs["access-key"]
		attrs["secret-key"] = credentialAttrs["secret-key"]
	default:
		return nil, errors.NotSupportedf("%q auth-type", authType)
	}

	// Set the default block-storage source.
	if _, ok := args.Config.StorageDefaultBlockSource(); !ok {
		attrs[config.StorageDefaultBlockSourceKey] = EBS_ProviderType
	}

	cfg, err := args.Config.Apply(attrs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return cfg, nil
}

// PrepareForBootstrap is specified in the EnvironProvider interface.
func (p environProvider) PrepareForBootstrap(
	ctx environs.BootstrapContext,
	cfg *config.Config,
) (environs.Environ, error) {
	e, err := p.Open(cfg)
	if err != nil {
		return nil, err
	}

	env := e.(*environ)
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(env); err != nil {
			return nil, err
		}
	}

	apiClient, ecfg := env.ec2(), env.ecfg()
	region, vpcID, forceVPCID := ecfg.region(), ecfg.vpcID(), ecfg.forceVPCID()
	if err := validateBootstrapVPC(apiClient, region, vpcID, forceVPCID, ctx); err != nil {
		return nil, errors.Trace(err)
	}

	return e, nil
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
		Region:        region,
		Endpoint:      ec2Region.EC2Endpoint,
		Architectures: arch.AllSupportedArches,
	}, nil
}

// SecretAttrs is specified in the EnvironProvider interface.
func (environProvider) SecretAttrs(cfg *config.Config) (map[string]string, error) {
	m := make(map[string]string)
	ecfg, err := providerInstance.newConfig(cfg)
	if err != nil {
		return nil, err
	}
	m["access-key"] = ecfg.accessKey()
	m["secret-key"] = ecfg.secretKey()
	return m, nil
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
	_, err := e.ec2().AccountAttributes()
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
