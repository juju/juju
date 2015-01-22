// Copyright 2011-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"gopkg.in/amz.v2/ec2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/juju/arch"
)

var logger = loggo.GetLogger("juju.provider.ec2")

func init() {
	environs.RegisterProvider("ec2", environProvider{})
}

type environProvider struct{}

var providerInstance environProvider

func (environProvider) BoilerplateConfig() string {
	return boilerplateConfig[1:]
}

func (p environProvider) Open(cfg *config.Config) (environs.Environ, error) {
	logger.Infof("opening environment %q", cfg.Name())
	e := new(environ)
	e.name = cfg.Name()
	err := e.SetConfig(cfg)
	if err != nil {
		return nil, err
	}
	return e, nil
}

func (p environProvider) Prepare(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	attrs := cfg.UnknownAttrs()
	if _, ok := attrs["control-bucket"]; !ok {
		uuid, err := utils.NewUUID()
		if err != nil {
			return nil, err
		}
		attrs["control-bucket"] = fmt.Sprintf("%x", uuid.Raw())
	}
	cfg, err := cfg.Apply(attrs)
	if err != nil {
		return nil, err
	}
	e, err := p.Open(cfg)
	if err != nil {
		return nil, err
	}
	if ctx.ShouldVerifyCredentials() {
		if err := verifyCredentials(e.(*environ)); err != nil {
			return nil, err
		}
	}
	return e, nil
}

func (environProvider) Validate(cfg, old *config.Config) (valid *config.Config, err error) {
	newEcfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid EC2 provider config: %v", err)
	}
	return cfg.Apply(newEcfg.attrs)
}

// MetadataLookupParams returns parameters which are used to query image metadata to
// find matching image information.
func (p environProvider) MetadataLookupParams(region string) (*simplestreams.MetadataLookupParams, error) {
	if region == "" {
		fmt.Errorf("region must be specified")
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
