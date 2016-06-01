// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"fmt"

	"github.com/juju/schema"
	"gopkg.in/amz.v3/aws"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{
	"access-key": {
		Description: "The EC2 access key",
		EnvVar:      "AWS_ACCESS_KEY_ID",
		Type:        environschema.Tstring,
		Mandatory:   true,
		Group:       environschema.AccountGroup,
	},
	"secret-key": {
		Description: "The EC2 secret key",
		EnvVar:      "AWS_SECRET_ACCESS_KEY",
		Type:        environschema.Tstring,
		Mandatory:   true,
		Secret:      true,
		Group:       environschema.AccountGroup,
	},
	"region": {
		Description: "The EC2 region to use",
		Type:        environschema.Tstring,
	},
	"vpc-id": {
		Description: "Use a specific AWS VPC ID (optional). When not specified, Juju requires a default VPC or EC2-Classic features to be available for the account/region.",
		Example:     "vpc-a1b2c3d4",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
		Immutable:   true,
	},
	"vpc-id-force": {
		Description: "Force Juju to use the AWS VPC ID specified with vpc-id, when it fails the minimum validation criteria. Not accepted without vpc-id",
		Type:        environschema.Tbool,
		Group:       environschema.AccountGroup,
		Immutable:   true,
	},
}

var configFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var configDefaults = schema.Defaults{
	"access-key":   "",
	"secret-key":   "",
	"region":       "us-east-1",
	"vpc-id":       "",
	"vpc-id-force": false,
}

type environConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *environConfig) region() string {
	return c.attrs["region"].(string)
}

func (c *environConfig) accessKey() string {
	return c.attrs["access-key"].(string)
}

func (c *environConfig) secretKey() string {
	return c.attrs["secret-key"].(string)
}

func (c *environConfig) vpcID() string {
	return c.attrs["vpc-id"].(string)
}

func (c *environConfig) forceVPCID() bool {
	return c.attrs["vpc-id-force"].(bool)
}

func (p environProvider) newConfig(cfg *config.Config) (*environConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &environConfig{valid, valid.UnknownAttrs()}, nil
}

// Schema returns the configuration schema for an environment.
func (environProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
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
			return nil, fmt.Errorf("model has no access-key or secret-key")
		}
		ecfg.attrs["access-key"] = auth.AccessKey
		ecfg.attrs["secret-key"] = auth.SecretKey
	}

	if _, ok := aws.Regions[ecfg.region()]; !ok {
		return nil, fmt.Errorf("invalid region name %q", ecfg.region())
	}

	if vpcID := ecfg.vpcID(); isVPCIDSetButInvalid(vpcID) {
		return nil, fmt.Errorf("vpc-id: %q is not a valid AWS VPC ID", vpcID)
	} else if !isVPCIDSet(vpcID) && ecfg.forceVPCID() {
		return nil, fmt.Errorf("cannot use vpc-id-force without specifying vpc-id as well")
	}

	if old != nil {
		attrs := old.UnknownAttrs()
		if region, _ := attrs["region"].(string); ecfg.region() != region {
			return nil, fmt.Errorf("cannot change region from %q to %q", region, ecfg.region())
		}

		if vpcID, _ := attrs["vpc-id"].(string); vpcID != ecfg.vpcID() {
			return nil, fmt.Errorf("cannot change vpc-id from %q to %q", vpcID, ecfg.vpcID())
		}

		if forceVPCID, _ := attrs["vpc-id-force"].(bool); forceVPCID != ecfg.forceVPCID() {
			return nil, fmt.Errorf("cannot change vpc-id-force from %v to %v", forceVPCID, ecfg.forceVPCID())
		}
	}

	// ssl-hostname-verification cannot be disabled
	if !ecfg.SSLHostnameVerification() {
		return nil, fmt.Errorf("disabling ssh-hostname-verification is not supported")
	}
	return ecfg, nil
}
