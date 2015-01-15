// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/schema"
	"gopkg.in/amz.v2/aws"

	"github.com/juju/juju/environs/config"
)

const boilerplateConfig = `
# https://juju.ubuntu.com/docs/config-aws.html
amazon:
    type: ec2

    # region specifies the EC2 region. It defaults to us-east-1.
    #
    # region: us-east-1

    # access-key holds the EC2 access key. It defaults to the
    # environment variable AWS_ACCESS_KEY_ID.
    #
    # access-key: <secret>

    # secret-key holds the EC2 secret key. It defaults to the
    # environment variable AWS_SECRET_ACCESS_KEY.
    #
    # secret-key: <secret>

    # image-stream chooses a simplestreams stream from which to select
    # OS images, for example daily or released images (or any other stream
    # available on simplestreams).
    #
    # image-stream: "released"

    # agent-stream chooses a simplestreams stream from which to select tools,
    # for example released or proposed tools (or any other stream available
    # on simplestreams).
    #
    # agent-stream: "released"

    # Whether or not to refresh the list of available updates for an
    # OS. The default option of true is recommended for use in
    # production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-refresh-update: true

    # Whether or not to perform OS upgrades when machines are
    # provisioned. The default option of true is recommended for use
    # in production systems, but disabling this can speed up local
    # deployments for development or testing.
    #
    # enable-os-upgrade: true

`

var configFields = schema.Fields{
	"access-key":     schema.String(),
	"secret-key":     schema.String(),
	"region":         schema.String(),
	"control-bucket": schema.String(),
}

var configDefaults = schema.Defaults{
	"access-key": "",
	"secret-key": "",
	"region":     "us-east-1",
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) region() string {
	return c.attrs["region"].(string)
}

func (c *environConfig) controlBucket() string {
	return c.attrs["control-bucket"].(string)
}

func (c *environConfig) accessKey() string {
	return c.attrs["access-key"].(string)
}

func (c *environConfig) secretKey() string {
	return c.attrs["secret-key"].(string)
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

func validateConfig(cfg, old *config.Config) (*environConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(configFields, configDefaults)
	if err != nil {
		return nil, err
	}
	ecfg := &environConfig{cfg, validated}

	if ecfg.accessKey() == "" || ecfg.secretKey() == "" {
		auth, err := aws.EnvAuth()
		if err != nil || ecfg.accessKey() != "" || ecfg.secretKey() != "" {
			return nil, fmt.Errorf("environment has no access-key or secret-key")
		}
		ecfg.attrs["access-key"] = auth.AccessKey
		ecfg.attrs["secret-key"] = auth.SecretKey
	}
	if _, ok := aws.Regions[ecfg.region()]; !ok {
		return nil, fmt.Errorf("invalid region name %q", ecfg.region())
	}

	if old != nil {
		attrs := old.UnknownAttrs()
		if region, _ := attrs["region"].(string); ecfg.region() != region {
			return nil, fmt.Errorf("cannot change region from %q to %q", region, ecfg.region())
		}
		if bucket, _ := attrs["control-bucket"].(string); ecfg.controlBucket() != bucket {
			return nil, fmt.Errorf("cannot change control-bucket from %q to %q", bucket, ecfg.controlBucket())
		}
	}

	// ssl-hostname-verification cannot be disabled
	if !ecfg.SSLHostnameVerification() {
		return nil, fmt.Errorf("disabling ssh-hostname-verification is not supported")
	}
	return ecfg, nil
}
