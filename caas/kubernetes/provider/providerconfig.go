// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"context"
	"fmt"

	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	k8sconstants "github.com/juju/juju/caas/kubernetes/provider/constants"
	"github.com/juju/juju/environs/config"
)

var configSchema = environschema.Fields{
	k8sconstants.WorkloadStorageKey: {
		Description: "The preferred storage class used to provision workload storage.",
		Type:        environschema.Tstring,
		Group:       environschema.AccountGroup,
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
	k8sconstants.WorkloadStorageKey: "",
}

type brokerConfig struct {
	*config.Config
	attrs map[string]interface{}
}

func (p kubernetesEnvironProvider) Validate(ctx context.Context, cfg, old *config.Config) (*config.Config, error) {
	newCfg, err := validateConfig(ctx, cfg, old)
	if err != nil {
		return nil, fmt.Errorf("invalid k8s provider config: %v", err)
	}
	return newCfg.Apply(newCfg.attrs)
}

func (p kubernetesEnvironProvider) newConfig(ctx context.Context, cfg *config.Config) (*brokerConfig, error) {
	valid, err := p.Validate(ctx, cfg, nil)
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

func validateConfig(ctx context.Context, cfg, old *config.Config) (*brokerConfig, error) {
	// Check for valid changes for the base config values.
	if err := config.Validate(ctx, cfg, old); err != nil {
		return nil, err
	}
	validated, err := cfg.ValidateUnknownAttrs(providerConfigFields, providerConfigDefaults)
	if err != nil {
		return nil, err
	}

	bcfg := &brokerConfig{cfg, validated}
	return bcfg, nil
}
