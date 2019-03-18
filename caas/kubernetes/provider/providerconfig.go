// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"fmt"

	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/environs/config"
)

const (
	WorkloadStorageKey = "workload-storage"
	OperatorStorageKey = "operator-storage"
)

var configSchema = environschema.Fields{
	WorkloadStorageKey: {
		Description: "The storage class used to provision workload storage.",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
	},
	OperatorStorageKey: {
		Description: "The storage class used to provision operator storage.",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
		Immutable:   true,
	},
}

var providerConfigFields = func() schema.Fields {
	fs, _, err := configSchema.ValidationSchema()
	if err != nil {
		panic(err)
	}
	return fs
}()

var providerConfigDefaults = schema.Defaults{
	WorkloadStorageKey: "",
	OperatorStorageKey: "",
}

type brokerConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (c *brokerConfig) storage() string {
	return c.attrs[WorkloadStorageKey].(string)
}

func (c *brokerConfig) operatorStorage() string {
	return c.attrs[OperatorStorageKey].(string)
}

func (p kubernetesEnvironProvider) Validate(cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := validateConfig(cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid k8s provider config: %v", err)
	}
	return newCfg.Apply(newCfg.attrs)
}

func (p kubernetesEnvironProvider) newConfig(cfg *config.Config) (*brokerConfig, error) {
	valid, err := p.Validate(cfg, nil)
	if err != nil {
		return nil, err
	}
	return &brokerConfig{valid, valid.UnknownAttrs()}, nil
}

// Schema returns the configuration schema for an environment.
func (kubernetesEnvironProvider) Schema() environschema.Fields {
	fields, err := config.Schema(configSchema)
	if err != nil {
		panic(err)
	}
	return fields
}

// ConfigSchema returns extra config attributes specific
// to this provider only.
func (p kubernetesEnvironProvider) ConfigSchema() schema.Fields {
	return providerConfigFields
}

// ConfigDefaults returns the default values for the
// provider specific config attributes.
func (p kubernetesEnvironProvider) ConfigDefaults() schema.Defaults {
	return providerConfigDefaults
}

func validateConfig(cfg, old *config.Config) (*brokerConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(providerConfigFields, providerConfigDefaults)
	if err != nil {
		return nil, err
	}

	bcfg := &brokerConfig{cfg, validated}
	return bcfg, nil
}
